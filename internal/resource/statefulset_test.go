package resource_test

import (
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	rabbitmqv1beta1 "github.com/pivotal/rabbitmq-for-kubernetes/api/v1beta1"
	"github.com/pivotal/rabbitmq-for-kubernetes/internal/resource"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	k8sresource "k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	defaultscheme "k8s.io/client-go/kubernetes/scheme"
)

var _ = Describe("StatefulSet", func() {
	var (
		instance rabbitmqv1beta1.RabbitmqCluster
		scheme   *runtime.Scheme
		cluster  *resource.RabbitmqResourceBuilder
		sts      *appsv1.StatefulSet
	)

	Context("Build", func() {
		BeforeEach(func() {
			instance = generateRabbitmqCluster()

			scheme = runtime.NewScheme()
			Expect(rabbitmqv1beta1.AddToScheme(scheme)).To(Succeed())
			Expect(defaultscheme.AddToScheme(scheme)).To(Succeed())
			cluster = &resource.RabbitmqResourceBuilder{
				Instance: &instance,
				Scheme:   scheme,
			}
			stsBuilder := cluster.StatefulSet()
			obj, _ := stsBuilder.Build()
			sts = obj.(*appsv1.StatefulSet)
		})

		It("sets the right service name", func() {
			Expect(sts.Spec.ServiceName).To(Equal(instance.ChildResourceName("headless")))
		})
		It("sets replicas", func() {
			Expect(*sts.Spec.Replicas).To(Equal(int32(1)))
		})

		It("adds the correct labels on the statefulset", func() {
			labels := sts.Labels
			Expect(labels["app.kubernetes.io/name"]).To(Equal(instance.Name))
			Expect(labels["app.kubernetes.io/component"]).To(Equal("rabbitmq"))
			Expect(labels["app.kubernetes.io/part-of"]).To(Equal("pivotal-rabbitmq"))
		})

		It("has resources requirements on the init container", func() {
			resources := sts.Spec.Template.Spec.InitContainers[0].Resources
			Expect(resources.Requests["cpu"]).To(Equal(k8sresource.MustParse("100m")))
			Expect(resources.Requests["memory"]).To(Equal(k8sresource.MustParse("500Mi")))
			Expect(resources.Limits["cpu"]).To(Equal(k8sresource.MustParse("100m")))
			Expect(resources.Limits["memory"]).To(Equal(k8sresource.MustParse("500Mi")))
		})

		It("adds the correct name with naming conventions", func() {
			expectedName := instance.ChildResourceName("server")
			Expect(sts.Name).To(Equal(expectedName))
		})

		It("specifies required Container Ports", func() {
			requiredContainerPorts := []int32{4369, 5672, 15672, 15692}
			var actualContainerPorts []int32

			container := extractContainer(sts.Spec.Template.Spec.Containers, "rabbitmq")
			for _, port := range container.Ports {
				actualContainerPorts = append(actualContainerPorts, port.ContainerPort)
			}

			Expect(actualContainerPorts).Should(ConsistOf(requiredContainerPorts))
		})

		It("uses required Environment Variables", func() {
			requiredEnvVariables := []corev1.EnvVar{
				{
					Name:  "RABBITMQ_ENABLED_PLUGINS_FILE",
					Value: "/opt/server-conf/enabled_plugins",
				},
				{
					Name:  "RABBITMQ_DEFAULT_PASS_FILE",
					Value: "/opt/rabbitmq-secret/password",
				},
				{
					Name:  "RABBITMQ_DEFAULT_USER_FILE",
					Value: "/opt/rabbitmq-secret/username",
				},
				{
					Name:  "RABBITMQ_MNESIA_BASE",
					Value: "/var/lib/rabbitmq/db",
				},
				{
					Name: "MY_POD_NAME",
					ValueFrom: &corev1.EnvVarSource{
						FieldRef: &corev1.ObjectFieldSelector{
							FieldPath:  "metadata.name",
							APIVersion: "v1",
						},
					},
				},
				{
					Name: "MY_POD_NAMESPACE",
					ValueFrom: &corev1.EnvVarSource{
						FieldRef: &corev1.ObjectFieldSelector{
							FieldPath:  "metadata.namespace",
							APIVersion: "v1",
						},
					},
				},
				{
					Name:  "K8S_SERVICE_NAME",
					Value: instance.ChildResourceName("headless"),
				},
				{
					Name:  "RABBITMQ_USE_LONGNAME",
					Value: "true",
				},
				{
					Name:  "RABBITMQ_NODENAME",
					Value: "rabbit@$(MY_POD_NAME).$(K8S_SERVICE_NAME).$(MY_POD_NAMESPACE).svc.cluster.local",
				},
				{
					Name:  "K8S_HOSTNAME_SUFFIX",
					Value: ".$(K8S_SERVICE_NAME).$(MY_POD_NAMESPACE).svc.cluster.local",
				},
			}

			container := extractContainer(sts.Spec.Template.Spec.Containers, "rabbitmq")
			Expect(container.Env).Should(ConsistOf(requiredEnvVariables))
		})

		It("creates required Volume Mounts for the rabbitmq container", func() {
			container := extractContainer(sts.Spec.Template.Spec.Containers, "rabbitmq")
			Expect(container.VolumeMounts).Should(ConsistOf(
				corev1.VolumeMount{
					Name:      "server-conf",
					MountPath: "/opt/server-conf/",
				},
				corev1.VolumeMount{
					Name:      "rabbitmq-admin",
					MountPath: "/opt/rabbitmq-secret/",
				},
				corev1.VolumeMount{
					Name:      "persistence",
					MountPath: "/var/lib/rabbitmq/db/",
				},
				corev1.VolumeMount{
					Name:      "rabbitmq-etc",
					MountPath: "/etc/rabbitmq/",
				},
				corev1.VolumeMount{
					Name:      "rabbitmq-erlang-cookie",
					MountPath: "/var/lib/rabbitmq/",
				},
			))
		})

		It("defines the expected volumes", func() {
			Expect(sts.Spec.Template.Spec.Volumes).Should(ConsistOf(
				corev1.Volume{
					Name: "rabbitmq-admin",
					VolumeSource: corev1.VolumeSource{
						Secret: &corev1.SecretVolumeSource{
							SecretName: instance.ChildResourceName("admin"),
							Items: []corev1.KeyToPath{
								{
									Key:  "username",
									Path: "username",
								},
								{
									Key:  "password",
									Path: "password",
								},
							},
						},
					},
				},
				corev1.Volume{
					Name: "server-conf",
					VolumeSource: corev1.VolumeSource{
						ConfigMap: &corev1.ConfigMapVolumeSource{
							LocalObjectReference: corev1.LocalObjectReference{
								Name: instance.ChildResourceName("server-conf"),
							},
						},
					},
				},
				corev1.Volume{
					Name: "rabbitmq-etc",
					VolumeSource: corev1.VolumeSource{
						EmptyDir: &corev1.EmptyDirVolumeSource{},
					},
				},
				corev1.Volume{
					Name: "rabbitmq-erlang-cookie",
					VolumeSource: corev1.VolumeSource{
						EmptyDir: &corev1.EmptyDirVolumeSource{},
					},
				},
				corev1.Volume{
					Name: "erlang-cookie-secret",
					VolumeSource: corev1.VolumeSource{
						Secret: &corev1.SecretVolumeSource{
							SecretName: instance.ChildResourceName("erlang-cookie"),
						},
					},
				},
			))
		})

		It("uses the correct service account", func() {
			Expect(sts.Spec.Template.Spec.ServiceAccountName).To(Equal(instance.ChildResourceName("server")))
		})

		It("does mount the service account in its pods", func() {
			Expect(*sts.Spec.Template.Spec.AutomountServiceAccountToken).To(BeTrue())
		})

		It("creates the required SecurityContext", func() {
			rmqGID, rmqUID := int64(999), int64(999)

			expectedPodSecurityContext := &corev1.PodSecurityContext{
				FSGroup:    &rmqGID,
				RunAsGroup: &rmqGID,
				RunAsUser:  &rmqUID,
			}

			Expect(sts.Spec.Template.Spec.SecurityContext).To(Equal(expectedPodSecurityContext))
		})

		It("defines a Readiness Probe", func() {
			container := extractContainer(sts.Spec.Template.Spec.Containers, "rabbitmq")
			actualProbeCommand := container.ReadinessProbe.Handler.Exec.Command
			Expect(actualProbeCommand).To(Equal([]string{"/bin/sh", "-c", "rabbitmq-diagnostics check_port_connectivity"}))
		})

		It("templates the correct InitContainer", func() {
			initContainers := sts.Spec.Template.Spec.InitContainers
			Expect(len(initContainers)).To(Equal(1))

			container := extractContainer(initContainers, "copy-config")
			Expect(container.Command).To(Equal([]string{
				"sh", "-c", "cp /tmp/rabbitmq/rabbitmq.conf /etc/rabbitmq/rabbitmq.conf && echo '' >> /etc/rabbitmq/rabbitmq.conf ; " +
					"cp /tmp/erlang-cookie-secret/.erlang.cookie /var/lib/rabbitmq/.erlang.cookie " +
					"&& chown 999:999 /var/lib/rabbitmq/.erlang.cookie " +
					"&& chmod 600 /var/lib/rabbitmq/.erlang.cookie",
			}))

			Expect(container.VolumeMounts).Should(ConsistOf(
				corev1.VolumeMount{
					Name:      "server-conf",
					MountPath: "/tmp/rabbitmq/",
				},
				corev1.VolumeMount{
					Name:      "rabbitmq-etc",
					MountPath: "/etc/rabbitmq/",
				},
				corev1.VolumeMount{
					Name:      "rabbitmq-erlang-cookie",
					MountPath: "/var/lib/rabbitmq/",
				},
				corev1.VolumeMount{
					Name:      "erlang-cookie-secret",
					MountPath: "/tmp/erlang-cookie-secret/",
				},
			))

			Expect(container.Image).To(Equal("rabbitmq-image-from-cr"))
		})

		It("adds the correct labels on the rabbitmq pods", func() {
			labels := sts.Spec.Template.ObjectMeta.Labels
			Expect(labels["app.kubernetes.io/name"]).To(Equal(instance.Name))
			Expect(labels["app.kubernetes.io/component"]).To(Equal("rabbitmq"))
			Expect(labels["app.kubernetes.io/part-of"]).To(Equal("pivotal-rabbitmq"))
		})

		It("adds the correct label selector", func() {
			labels := sts.Spec.Selector.MatchLabels
			Expect(labels["app.kubernetes.io/name"]).To(Equal(instance.Name))
		})

		It("adds the required terminationGracePeriodSeconds", func() {
			gracePeriodSeconds := sts.Spec.Template.Spec.TerminationGracePeriodSeconds
			expectedGracePeriodSeconds := int64(150)
			Expect(gracePeriodSeconds).To(Equal(&expectedGracePeriodSeconds))
		})

		It("creates the affinity rule as provided in the instance", func() {
			affinity := &corev1.Affinity{
				NodeAffinity: &corev1.NodeAffinity{
					RequiredDuringSchedulingIgnoredDuringExecution: &corev1.NodeSelector{
						NodeSelectorTerms: []corev1.NodeSelectorTerm{
							{
								MatchExpressions: []corev1.NodeSelectorRequirement{
									{
										Key:      "foo",
										Operator: "Exists",
										Values:   nil,
									},
								},
							},
						},
					},
				},
			}
			instance.Spec.Affinity = affinity
			cluster = &resource.RabbitmqResourceBuilder{
				Instance: &instance,
				Scheme:   scheme,
			}
			stsBuilder := cluster.StatefulSet()
			obj, _ := stsBuilder.Build()
			sts = obj.(*appsv1.StatefulSet)
			Expect(sts.Spec.Template.Spec.Affinity).To(Equal(affinity))
		})

		Context("Tolerations", func() {
			It("creates the tolerations specified", func() {
				tolerations := []corev1.Toleration{
					{
						Key:      "key",
						Operator: "equals",
						Value:    "value",
						Effect:   "NoSchedule",
					},
				}

				instance.Spec.Tolerations = tolerations
				cluster = &resource.RabbitmqResourceBuilder{
					Instance: &instance,
					Scheme:   scheme,
				}
				stsBuilder := cluster.StatefulSet()
				obj, _ := stsBuilder.Build()
				sts = obj.(*appsv1.StatefulSet)
				Expect(sts.Spec.Template.Spec.Tolerations).To(Equal(tolerations))
			})
		})

		Context("PVC template", func() {
			It("creates the required PersistentVolumeClaim", func() {
				truth := true
				q, _ := k8sresource.ParseQuantity("10Gi")

				expectedPersistentVolumeClaim := corev1.PersistentVolumeClaim{
					ObjectMeta: v1.ObjectMeta{
						Name:      "persistence",
						Namespace: instance.Namespace,
						Labels: map[string]string{
							"app.kubernetes.io/name":      instance.Name,
							"app.kubernetes.io/component": "rabbitmq",
							"app.kubernetes.io/part-of":   "pivotal-rabbitmq",
						},
						OwnerReferences: []v1.OwnerReference{
							{
								APIVersion:         "rabbitmq.pivotal.io/v1beta1",
								Kind:               "RabbitmqCluster",
								Name:               instance.Name,
								UID:                "",
								Controller:         &truth,
								BlockOwnerDeletion: &truth,
							},
						},
						Annotations: map[string]string{},
					},
					Spec: corev1.PersistentVolumeClaimSpec{
						AccessModes: []corev1.PersistentVolumeAccessMode{corev1.ReadWriteOnce},
						Resources: corev1.ResourceRequirements{
							Requests: map[corev1.ResourceName]k8sresource.Quantity{
								corev1.ResourceStorage: q,
							},
						},
					},
				}

				actualPersistentVolumeClaim := sts.Spec.VolumeClaimTemplates[0]
				Expect(actualPersistentVolumeClaim).To(Equal(expectedPersistentVolumeClaim))
			})

			It("references the storageclassname when specified", func() {
				storageClassName := "my-storage-class"
				instance.Spec.Persistence.StorageClassName = &storageClassName
				cluster = &resource.RabbitmqResourceBuilder{
					Instance: &instance,
					Scheme:   scheme,
				}
				stsBuilder := cluster.StatefulSet()
				obj, _ := stsBuilder.Build()
				sts = obj.(*appsv1.StatefulSet)
				Expect(*sts.Spec.VolumeClaimTemplates[0].Spec.StorageClassName).To(Equal("my-storage-class"))
			})

			It("creates the PersistentVolume template according to configurations in the  instance", func() {
				storage := k8sresource.MustParse("21Gi")
				instance.Spec.Persistence.Storage = &storage
				cluster = &resource.RabbitmqResourceBuilder{
					Instance: &instance,
					Scheme:   scheme,
				}
				stsBuilder := cluster.StatefulSet()
				obj, _ := stsBuilder.Build()
				sts = obj.(*appsv1.StatefulSet)
				q, _ := k8sresource.ParseQuantity("21Gi")
				Expect(sts.Spec.VolumeClaimTemplates[0].Spec.Resources.Requests["storage"]).To(Equal(q))
			})
		})

		Context("resources requirements", func() {

			It("sets StatefulSet resource requirements", func() {
				instance.Spec.Resources = &corev1.ResourceRequirements{
					Requests: corev1.ResourceList{
						corev1.ResourceCPU:    k8sresource.MustParse("10m"),
						corev1.ResourceMemory: k8sresource.MustParse("3Gi"),
					},
					Limits: corev1.ResourceList{
						corev1.ResourceCPU:    k8sresource.MustParse("11m"),
						corev1.ResourceMemory: k8sresource.MustParse("4Gi"),
					},
				}

				cluster = &resource.RabbitmqResourceBuilder{
					Instance: &instance,
					Scheme:   scheme,
				}

				stsBuilder := cluster.StatefulSet()
				obj, _ := stsBuilder.Build()
				sts = obj.(*appsv1.StatefulSet)
				expectedCPURequest, _ := k8sresource.ParseQuantity("10m")
				expectedMemoryRequest, _ := k8sresource.ParseQuantity("3Gi")
				expectedCPULimit, _ := k8sresource.ParseQuantity("11m")
				expectedMemoryLimit, _ := k8sresource.ParseQuantity("4Gi")

				container := extractContainer(sts.Spec.Template.Spec.Containers, "rabbitmq")
				Expect(container.Resources.Requests[corev1.ResourceCPU]).To(Equal(expectedCPURequest))
				Expect(container.Resources.Requests[corev1.ResourceMemory]).To(Equal(expectedMemoryRequest))
				Expect(container.Resources.Limits[corev1.ResourceCPU]).To(Equal(expectedCPULimit))
				Expect(container.Resources.Limits[corev1.ResourceMemory]).To(Equal(expectedMemoryLimit))
			})

			It("does not set any resource requirements if empty maps are provided in the CR", func() {
				instance.Spec.Resources = &corev1.ResourceRequirements{
					Requests: corev1.ResourceList{},
					Limits:   corev1.ResourceList{},
				}

				cluster = &resource.RabbitmqResourceBuilder{
					Instance: &instance,
					Scheme:   scheme,
				}

				stsBuilder := cluster.StatefulSet()
				obj, _ := stsBuilder.Build()
				sts = obj.(*appsv1.StatefulSet)

				container := extractContainer(sts.Spec.Template.Spec.Containers, "rabbitmq")
				Expect(len(container.Resources.Requests)).To(Equal(0))
				Expect(len(container.Resources.Limits)).To(Equal(0))
			})
		})

		When("configures private image", func() {
			It("uses the instance ImagePullSecret and image reference when provided", func() {
				instance.Spec.Image = "my-private-repo/rabbitmq:latest"
				instance.Spec.ImagePullSecret = "my-great-secret"
				cluster = &resource.RabbitmqResourceBuilder{
					Instance: &instance,
					Scheme:   scheme,
				}

				stsBuilder := cluster.StatefulSet()
				obj, _ := stsBuilder.Build()
				sts = obj.(*appsv1.StatefulSet)
				container := extractContainer(sts.Spec.Template.Spec.Containers, "rabbitmq")
				Expect(container.Image).To(Equal("my-private-repo/rabbitmq:latest"))
				Expect(sts.Spec.Template.Spec.ImagePullSecrets).To(ConsistOf(corev1.LocalObjectReference{Name: "my-great-secret"}))
			})
		})

		It("sets the replica count of the StatefulSet to the instance value", func() {
			instance.Spec.Replicas = 3
			cluster = &resource.RabbitmqResourceBuilder{
				Instance: &instance,
				Scheme:   scheme,
			}
			stsBuilder := cluster.StatefulSet()
			obj, _ := stsBuilder.Build()
			sts = obj.(*appsv1.StatefulSet)
			Expect(*sts.Spec.Replicas).To(Equal(int32(3)))
		})
	})

	Context("Build with instance labels", func() {
		BeforeEach(func() {
			instance = generateRabbitmqCluster()
			instance.Namespace = "foo"
			instance.Name = "foo"
			instance.Labels = map[string]string{
				"app.kubernetes.io/foo": "bar",
				"foo":                   "bar",
				"rabbitmq":              "is-great",
				"foo/app.kubernetes.io": "edgecase",
			}

			scheme = runtime.NewScheme()
			Expect(rabbitmqv1beta1.AddToScheme(scheme)).To(Succeed())
			Expect(defaultscheme.AddToScheme(scheme)).To(Succeed())

			cluster = &resource.RabbitmqResourceBuilder{
				Instance: &instance,
				Scheme:   scheme,
			}
		})

		It("has the labels from the instance on the statefulset", func() {
			stsBuilder := cluster.StatefulSet()
			obj, _ := stsBuilder.Build()
			sts = obj.(*appsv1.StatefulSet)
			testLabels(sts.Labels)
		})

		It("has the labels from the instance on the pod", func() {
			stsBuilder := cluster.StatefulSet()
			obj, _ := stsBuilder.Build()
			sts = obj.(*appsv1.StatefulSet)
			podTemplate := sts.Spec.Template
			testLabels(podTemplate.Labels)
		})
	})

	Context("Build with instance annotations", func() {
		BeforeEach(func() {
			instance = generateRabbitmqCluster()
			instance.Namespace = "foo"
			instance.Name = "foo"
			instance.Annotations = map[string]string{
				"my-annotation":              "i-like-this",
				"kubernetes.io/name":         "i-do-not-like-this",
				"kubectl.kubernetes.io/name": "i-do-not-like-this",
				"k8s.io/name":                "i-do-not-like-this",
			}

			scheme = runtime.NewScheme()
			Expect(rabbitmqv1beta1.AddToScheme(scheme)).To(Succeed())
			Expect(defaultscheme.AddToScheme(scheme)).To(Succeed())

			cluster = &resource.RabbitmqResourceBuilder{
				Instance: &instance,
				Scheme:   scheme,
			}
			stsBuilder := cluster.StatefulSet()
			obj, _ := stsBuilder.Build()
			sts = obj.(*appsv1.StatefulSet)
		})

		It("has the annotations from the instance on the StatefulSet", func() {
			expectedAnnotations := map[string]string{
				"my-annotation": "i-like-this",
			}

			Expect(sts.Annotations).To(Equal(expectedAnnotations))
		})

		It("has the annotations from the instance on the pod", func() {
			podTemplate := sts.Spec.Template
			expectedAnnotations := map[string]string{
				"my-annotation": "i-like-this",
			}
			Expect(podTemplate.Annotations).To(Equal(expectedAnnotations))
		})
	})

	Context("Update", func() {
		var (
			statefulSet                    *appsv1.StatefulSet
			stsBuilder                     *resource.StatefulSetBuilder
			existingLabels                 map[string]string
			existingAnnotations            map[string]string
			existingPodTemplateAnnotations map[string]string
			affinity                       *corev1.Affinity
		)

		BeforeEach(func() {
			scheme = runtime.NewScheme()
			Expect(rabbitmqv1beta1.AddToScheme(scheme)).To(Succeed())
			Expect(defaultscheme.AddToScheme(scheme)).To(Succeed())

			cluster = &resource.RabbitmqResourceBuilder{
				Instance: &instance,
				Scheme:   scheme,
			}
			existingLabels = map[string]string{
				"app.kubernetes.io/name":      instance.Name,
				"app.kubernetes.io/part-of":   "pivotal-rabbitmq",
				"this-was-the-previous-label": "should-be-deleted",
			}

			existingAnnotations = map[string]string{
				"this-was-the-previous-annotation": "should-be-preserved",
				"app.kubernetes.io/part-of":        "pivotal-rabbitmq",
				"app.k8s.io/something":             "something-amazing",
			}

			existingPodTemplateAnnotations = map[string]string{
				"this-was-the-previous-pod-anno": "should-be-preserved",
				"app.kubernetes.io/part-of":      "pivotal-rabbitmq-pod",
				"app.k8s.io/something":           "something-amazing-on-pod",
			}

			stsBuilder = cluster.StatefulSet()

			statefulSet = &appsv1.StatefulSet{
				ObjectMeta: metav1.ObjectMeta{
					Labels:      existingLabels,
					Annotations: existingAnnotations,
				},
				Spec: appsv1.StatefulSetSpec{
					Template: corev1.PodTemplateSpec{
						ObjectMeta: metav1.ObjectMeta{
							Labels:      existingLabels,
							Annotations: existingPodTemplateAnnotations,
						},
						Spec: corev1.PodSpec{
							InitContainers: []corev1.Container{{}},
							Containers:     []corev1.Container{{}},
						},
					},
				},
			}
		})

		It("creates the affinity rule as provided in the instance", func() {
			affinity = &corev1.Affinity{
				NodeAffinity: &corev1.NodeAffinity{
					RequiredDuringSchedulingIgnoredDuringExecution: &corev1.NodeSelector{
						NodeSelectorTerms: []corev1.NodeSelectorTerm{
							{
								MatchExpressions: []corev1.NodeSelectorRequirement{
									{
										Key:      "foo",
										Operator: "Exists",
										Values:   nil,
									},
								},
							},
						},
					},
				},
			}
			stsBuilder.Instance.Spec.Affinity = affinity

			Expect(stsBuilder.Update(statefulSet)).To(Succeed())
			Expect(statefulSet.Spec.Template.Spec.Affinity).To(Equal(affinity))
		})

		It("updates labels", func() {
			stsBuilder.Instance.Labels = map[string]string{
				"app.kubernetes.io/foo": "bar",
				"foo":                   "bar",
				"rabbitmq":              "is-great",
				"foo/app.kubernetes.io": "edgecase",
			}
			Expect(stsBuilder.Update(statefulSet)).To(Succeed())

			By("updating labels from the CR to the statefulset")
			testLabels(statefulSet.Labels)

			By("restoring the default labels")
			labels := statefulSet.Labels
			Expect(labels["app.kubernetes.io/name"]).To(Equal("foo"))
			Expect(labels["app.kubernetes.io/component"]).To(Equal("rabbitmq"))
			Expect(labels["app.kubernetes.io/part-of"]).To(Equal("pivotal-rabbitmq"))

			By("deleting the labels that are removed from the CR")
			Expect(stsBuilder.Update(statefulSet)).To(Succeed())
			Expect(statefulSet.Labels).NotTo(HaveKey("this-was-the-previous-label"))
		})

		It("updates annotations", func() {
			stsBuilder.Instance.Annotations = map[string]string{
				"my-annotation":              "i-like-this",
				"kubernetes.io/name":         "i-do-not-like-this",
				"kubectl.kubernetes.io/name": "i-do-not-like-this",
				"k8s.io/name":                "i-do-not-like-this",
			}
			Expect(stsBuilder.Update(statefulSet)).To(Succeed())

			expectedAnnotations := map[string]string{
				"my-annotation":                    "i-like-this",
				"this-was-the-previous-annotation": "should-be-preserved",
				"app.kubernetes.io/part-of":        "pivotal-rabbitmq",
				"app.k8s.io/something":             "something-amazing",
			}

			Expect(statefulSet.Annotations).To(Equal(expectedAnnotations))
		})

		It("updates tolerations", func() {
			newToleration := corev1.Toleration{
				Key:      "update",
				Operator: "equals",
				Value:    "works",
				Effect:   "NoSchedule",
			}
			stsBuilder.Instance.Spec.Tolerations = []corev1.Toleration{newToleration}
			Expect(stsBuilder.Update(statefulSet)).To(Succeed())

			Expect(statefulSet.Spec.Template.Spec.Tolerations).
				To(ConsistOf(newToleration))
		})

		It("updates the image pull secret; sets it back to default after deleting the configuration", func() {
			stsBuilder.Instance.Spec.ImagePullSecret = "my-shiny-new-secret"
			Expect(stsBuilder.Update(statefulSet)).To(Succeed())
			Expect(statefulSet.Spec.Template.Spec.ImagePullSecrets).To(ConsistOf(corev1.LocalObjectReference{Name: "my-shiny-new-secret"}))

			stsBuilder.Instance.Spec.ImagePullSecret = ""
			Expect(stsBuilder.Update(statefulSet)).To(Succeed())
			Expect(statefulSet.Spec.Template.Spec.ImagePullSecrets).To(BeEmpty())
		})

		Context("updates labels on pod", func() {
			BeforeEach(func() {
				statefulSet.Spec.Template.Labels = existingLabels
			})

			It("adds labels from the CR to the pod", func() {
				Expect(stsBuilder.Update(statefulSet)).To(Succeed())

				testLabels(statefulSet.Spec.Template.Labels)
			})

			It("restores the default labels", func() {
				Expect(stsBuilder.Update(statefulSet)).To(Succeed())

				labels := statefulSet.Spec.Template.Labels
				Expect(labels["app.kubernetes.io/name"]).To(Equal(instance.Name))
				Expect(labels["app.kubernetes.io/component"]).To(Equal("rabbitmq"))
				Expect(labels["app.kubernetes.io/part-of"]).To(Equal("pivotal-rabbitmq"))
			})

			It("deletes the labels that are removed from the CR", func() {
				Expect(stsBuilder.Update(statefulSet)).To(Succeed())

				Expect(statefulSet.Spec.Template.Labels).NotTo(HaveKey("this-was-the-previous-label"))
			})
		})

		Context("updates annotations on pod", func() {
			BeforeEach(func() {
				statefulSet.Annotations = existingAnnotations
				statefulSet.Spec.Template.Annotations = existingPodTemplateAnnotations
			})

			It("update annotations from the instance to the pod", func() {
				stsBuilder.Instance.Annotations = map[string]string{
					"my-annotation":              "i-like-this",
					"kubernetes.io/name":         "i-do-not-like-this",
					"kubectl.kubernetes.io/name": "i-do-not-like-this",
					"k8s.io/name":                "i-do-not-like-this",
				}

				Expect(stsBuilder.Update(statefulSet)).To(Succeed())
				expectedAnnotations := map[string]string{
					"my-annotation":                  "i-like-this",
					"app.kubernetes.io/part-of":      "pivotal-rabbitmq-pod",
					"this-was-the-previous-pod-anno": "should-be-preserved",
					"app.k8s.io/something":           "something-amazing-on-pod",
				}

				Expect(statefulSet.Spec.Template.Annotations).To(Equal(expectedAnnotations))
			})
		})
	})
})

func extractContainer(containers []corev1.Container, containerName string) corev1.Container {
	for _, container := range containers {
		if container.Name == containerName {
			return container
		}
	}

	return corev1.Container{}
}

func generateRabbitmqCluster() rabbitmqv1beta1.RabbitmqCluster {
	storage := k8sresource.MustParse("10Gi")
	return rabbitmqv1beta1.RabbitmqCluster{
		ObjectMeta: v1.ObjectMeta{
			Name:      "foo",
			Namespace: "foo",
		},
		Spec: rabbitmqv1beta1.RabbitmqClusterSpec{
			Replicas:        int32(1),
			Image:           "rabbitmq-image-from-cr",
			ImagePullSecret: "my-super-secret",
			Service: rabbitmqv1beta1.RabbitmqClusterServiceSpec{
				Type:        corev1.ServiceType("this-is-a-service"),
				Annotations: map[string]string{},
			},
			Persistence: rabbitmqv1beta1.RabbitmqClusterPersistenceSpec{
				StorageClassName: nil,
				Storage:          &storage,
			},
			Resources: &corev1.ResourceRequirements{
				Limits: map[corev1.ResourceName]k8sresource.Quantity{
					"cpu":    k8sresource.MustParse("16"),
					"memory": k8sresource.MustParse("16Gi"),
				},
				Requests: map[corev1.ResourceName]k8sresource.Quantity{
					"cpu":    k8sresource.MustParse("15"),
					"memory": k8sresource.MustParse("15Gi"),
				},
			},
			Affinity: &corev1.Affinity{
				NodeAffinity: &corev1.NodeAffinity{
					RequiredDuringSchedulingIgnoredDuringExecution: &corev1.NodeSelector{
						NodeSelectorTerms: []corev1.NodeSelectorTerm{
							corev1.NodeSelectorTerm{
								MatchExpressions: []corev1.NodeSelectorRequirement{
									{
										Key:      "somekey",
										Operator: "Equal",
										Values:   []string{"this-value"},
									},
								},
								MatchFields: nil,
							},
						},
					},
				},
			},
			Tolerations: []corev1.Toleration{
				corev1.Toleration{
					Key:      "mykey",
					Operator: "NotEqual",
					Value:    "myvalue",
					Effect:   "NoSchedule",
				},
			},
		},
	}
}
