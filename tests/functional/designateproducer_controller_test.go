/*
Copyright 2024.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package functional_test

import (
	"fmt"

	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2" //revive:disable:dot-imports
	. "github.com/onsi/gomega"    //revive:disable:dot-imports
	"github.com/openstack-k8s-operators/lib-common/modules/common/condition"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	// "sigs.k8s.io/controller-runtime/pkg/client"
	// revive:disable-next-line:dot-imports
	"github.com/openstack-k8s-operators/designate-operator/pkg/designate"
	. "github.com/openstack-k8s-operators/lib-common/modules/common/test/helpers"
	mariadbv1 "github.com/openstack-k8s-operators/mariadb-operator/api/v1beta1"
)

var _ = Describe("DesignateProducer controller", func() {
	var name string
	var spec map[string]any
	var designateProducerName types.NamespacedName
	var designateRedisName types.NamespacedName
	var transportURLSecretName types.NamespacedName

	BeforeEach(func() {
		name = fmt.Sprintf("designate-producer-%s", uuid.New().String())
		spec = GetDefaultDesignateProducerSpec()

		transportURLSecretName = types.NamespacedName{
			Namespace: namespace,
			Name:      RabbitmqSecretName,
		}

		designateProducerName = types.NamespacedName{
			Name:      name,
			Namespace: namespace,
		}

		designateRedisName = types.NamespacedName{
			Namespace: namespace,
			Name:      "designate-redis",
		}
	})
	When("a DesignateProducer instance is created", func() {
		BeforeEach(func() {
			DeferCleanup(th.DeleteInstance, CreateDesignateProducer(designateProducerName, spec))
		})

		It("should have the Status fields initialized", func() {
			designateProducer := GetDesignateProducer(designateProducerName)
			Expect(designateProducer.Status.ReadyCount).Should(Equal(int32(0)))
		})

		It("should have a finalizer", func() {
			// the reconciler loop adds the finalizer so we have to wait for
			// it to run
			Eventually(func() []string {
				return GetDesignateProducer(designateProducerName).Finalizers
			}, timeout, interval).Should(ContainElement("openstack.org/designateproducer"))
		})

		It("should have a expected default values", func() {
			designateProducer := GetDesignateProducer((designateProducerName))
			Expect(designateProducer.Spec.ServiceUser).Should(Equal("designate"))
			Expect(designateProducer.Spec.DatabaseAccount).Should(Equal("designate"))
			Expect(designateProducer.Spec.PasswordSelectors.Service).Should(Equal("DesignatePassword"))
		})

		It("should not create a secret", func() {
			secret := types.NamespacedName{
				Namespace: designateProducerName.Namespace,
				Name:      fmt.Sprintf("%s-%s", designateProducerName.Name, "config-data"),
			}
			th.AssertSecretDoesNotExist(secret)
		})
	})

	When("a proper secret is provided and TransportURL is created", func() {
		BeforeEach(func() {
			DeferCleanup(th.DeleteInstance, CreateDesignateProducer(designateProducerName, spec))
			DeferCleanup(k8sClient.Delete, ctx, CreateDesignateSecret(namespace))
			DeferCleanup(k8sClient.Delete, ctx, CreateTransportURLSecret(transportURLSecretName))
		})

		It("should not create a secret", func() {
			secret := types.NamespacedName{
				Namespace: designateProducerName.Namespace,
				Name:      fmt.Sprintf("%s-%s", designateProducerName.Name, "config-data"),
			}
			th.AssertSecretDoesNotExist(secret)
		})
	})

	When("config files are created", func() {
		var keystoneInternalEndpoint string
		var keystonePublicEndpoint string

		BeforeEach(func() {
			keystoneName := keystone.CreateKeystoneAPI(namespace)
			DeferCleanup(keystone.DeleteKeystoneAPI, keystoneName)
			keystoneInternalEndpoint = fmt.Sprintf("http://keystone-for-%s-internal", designateProducerName.Name)
			keystonePublicEndpoint = fmt.Sprintf("http://keystone-for-%s-public", designateProducerName.Name)
			SimulateKeystoneReady(keystoneName, keystonePublicEndpoint, keystoneInternalEndpoint)

			createAndSimulateRedis(designateRedisName)
			DeferCleanup(k8sClient.Delete, ctx, CreateDesignateSecret(namespace))
			DeferCleanup(k8sClient.Delete, ctx, CreateTransportURLSecret(transportURLSecretName))
			createOwnerSecrets(namespace)

			spec["customServiceConfig"] = "[DEFAULT]\ndebug=True\n"
			DeferCleanup(th.DeleteInstance, CreateDesignateProducer(designateProducerName, spec))

			mariaDBDatabaseName := mariadb.CreateMariaDBDatabase(namespace, designate.DatabaseCRName, mariadbv1.MariaDBDatabaseSpec{})
			mariaDBDatabase := mariadb.GetMariaDBDatabase(mariaDBDatabaseName)
			DeferCleanup(k8sClient.Delete, ctx, mariaDBDatabase)

			designateProducer := GetDesignateProducer(designateProducerName)
			apiMariaDBAccount, apiMariaDBSecret := mariadb.CreateMariaDBAccountAndSecret(
				types.NamespacedName{
					Namespace: namespace,
					Name:      designateProducer.Spec.DatabaseAccount,
				}, mariadbv1.MariaDBAccountSpec{})
			DeferCleanup(k8sClient.Delete, ctx, apiMariaDBAccount)
			DeferCleanup(k8sClient.Delete, ctx, apiMariaDBSecret)

			DeferCleanup(k8sClient.Delete, ctx, CreateNAD(types.NamespacedName{
				Name:      spec["designateNetworkAttachment"].(string),
				Namespace: namespace,
			}))
		})

		It("should be in state of having the input ready", func() {
			th.ExpectCondition(
				designateProducerName,
				ConditionGetterFunc(DesignateProducerConditionGetter),
				condition.InputReadyCondition,
				corev1.ConditionTrue,
			)
		})

		It("should set Service Config Ready Condition", func() {
			th.ExpectCondition(
				designateProducerName,
				ConditionGetterFunc(DesignateProducerConditionGetter),
				condition.ServiceConfigReadyCondition,
				corev1.ConditionTrue,
			)
		})

		// It("should create the designate.conf file in a Secret", func() {
		// 	// TODO(oschwart): remove below debug printing
		// 	secretList := &corev1.SecretList{}
		// 	listOpts := []client.ListOption{
		// 		client.InNamespace(namespace),
		// 	}
		// 	if err := k8sClient.List(ctx, secretList, listOpts...); err != nil {
		// 		return
		// 	}

		// 	fmt.Printf("\n=== Secrets in namespace %s ===\n", namespace)
		// 	for _, secret := range secretList.Items {
		// 		fmt.Printf("- Name: %s\n", secret.Name)
		// 	}
		// 	configData := th.GetSecret(
		// 		// configData := th.GetSecret(
		// 		types.NamespacedName{
		// 			Namespace: namespace,
		// 			Name:      fmt.Sprintf("%s-config-data", designateProducerName.Name)})
		// 	Expect(configData).ShouldNot(BeNil())
		// 	conf := string(configData.Data["designate.conf"])
		// 	// instance := GetDesignateProducer(designateProducerName)

		// 	// dbs := []struct {
		// 	// 	Name            string
		// 	// 	DatabaseAccount string
		// 	// 	Keyword         string
		// 	// }{
		// 	// 	{
		// 	// 		Name:            designate.DatabaseName,
		// 	// 		DatabaseAccount: instance.Spec.DatabaseAccount,
		// 	// 		Keyword:         "connection",
		// 	// 	},
		// 	// }

		// 	// for _, db := range dbs {
		// 	// 	databaseAccount := mariadb.GetMariaDBAccount(
		// 	// 		types.NamespacedName{
		// 	// 			Namespace: namespace,
		// 	// 			Name:      db.DatabaseAccount})
		// 	// 	databaseSecret := th.GetSecret(
		// 	// 		types.NamespacedName{
		// 	// 			Namespace: namespace,
		// 	// 			Name:      databaseAccount.Spec.Secret})

		// 	// 	Expect(conf).Should(
		// 	// 		ContainSubstring(
		// 	// 			fmt.Sprintf(
		// 	// 				"%s=mysql+pymysql://%s:%s@%s/%s?read_default_file=/etc/my.cnf",
		// 	// 				db.Keyword,
		// 	// 				databaseAccount.Spec.UserName,
		// 	// 				databaseSecret.Data[mariadbv1.DatabasePasswordSelector],
		// 	// 				instance.Spec.DatabaseHostname,
		// 	// 				db.Name)))
		// 	// }

		// 	Expect(conf).Should(
		// 		ContainSubstring(fmt.Sprintf(
		// 			"www_authenticate_uri=%s\n", keystonePublicEndpoint)))
		// 	// TBC: [keystone_authtoken].auth_url and [service_auth].auth_url differ?
		// 	Expect(conf).Should(
		// 		ContainSubstring(fmt.Sprintf(
		// 			"auth_url=%s\n", keystoneInternalEndpoint)))
		// 	Expect(conf).Should(
		// 		ContainSubstring(fmt.Sprintf(
		// 			"auth_url=%s\n", keystoneInternalEndpoint)))
		// 	// Expect(conf).Should(
		// 	// 	ContainSubstring(fmt.Sprintf(
		// 	// 		"username=%s\n", instance.Spec.ServiceUser)))

		// 	ospSecret := th.GetSecret(types.NamespacedName{
		// 		Name:      SecretName,
		// 		Namespace: namespace})
		// 	Expect(conf).Should(
		// 		ContainSubstring(fmt.Sprintf(
		// 			"\npassword=%s\n", string(ospSecret.Data["DesignatePassword"]))))

		// 	transportURLSecret := th.GetSecret(transportURLSecretName)
		// 	Expect(conf).Should(
		// 		ContainSubstring(fmt.Sprintf(
		// 			"transport_url=%s\n", string(transportURLSecret.Data["transport_url"]))))
		// })

		// It("should create a Secret with customServiceConfig input", func() {
		// 	configData := th.GetSecret(
		// 		types.NamespacedName{
		// 			Namespace: designateProducerName.Namespace,
		// 			Name:      fmt.Sprintf("%s-config-data", designateProducerName.Name)})
		// 	Expect(configData).ShouldNot(BeNil())
		// 	conf := string(configData.Data["custom.conf"])
		// 	Expect(conf).Should(
		// 		ContainSubstring("[DEFAULT]\ndebug=True\n"))
		// })
	})
})
