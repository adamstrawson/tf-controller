package controllers

import (
	"context"
	"time"

	infrav1 "github.com/chanwit/tf-controller/api/v1alpha1"
	sourcev1 "github.com/fluxcd/source-controller/api/v1beta1"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

// +kubebuilder:docs-gen:collapse=Imports

var _ = Describe("TF controller", func() {
	const (
		sourceName    = "test-tf-with-backend-no-output"
		terraformName = "helloworld-with-backend-no-outputs"
	)

	Context("When create a hello world TF object", func() {
		It("should run the TF hello world program from the BLOB and get the correct output as a secret", func() {
			ctx := context.Background()
			testEnvKubeConfigPath, err := findKubeConfig(testEnv)
			Expect(err).Should(BeNil())

			By("creating a new Git repository object")
			updatedTime := time.Now()
			testRepo := sourcev1.GitRepository{
				TypeMeta: metav1.TypeMeta{
					APIVersion: "source.toolkit.fluxcd.io/v1beta1",
					Kind:       "GitRepository",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name:      sourceName,
					Namespace: "flux-system",
				},
				Spec: sourcev1.GitRepositorySpec{
					URL: "https://github.com/openshift-fluxv2-poc/podinfo",
					Reference: &sourcev1.GitRepositoryRef{
						Branch: "master",
					},
					Interval:          metav1.Duration{Duration: time.Second * 30},
					GitImplementation: "go-git",
				},
			}
			Expect(k8sClient.Create(ctx, &testRepo)).Should(Succeed())

			By("setting the git repo status object, the URL, and the correct checksum")
			testRepo.Status = sourcev1.GitRepositoryStatus{
				ObservedGeneration: int64(1),
				Conditions: []metav1.Condition{
					{
						Type:               "Ready",
						Status:             metav1.ConditionTrue,
						LastTransitionTime: metav1.Time{Time: updatedTime},
						Reason:             "GitOperationSucceed",
						Message:            "Fetched revision: master/b8e362c206e3d0cbb7ed22ced771a0056455a2fb",
					},
				},
				URL: server.URL() + "/file.tar.gz",
				Artifact: &sourcev1.Artifact{
					Path:           "gitrepository/flux-system/test-tf-controller/b8e362c206e3d0cbb7ed22ced771a0056455a2fb.tar.gz",
					URL:            server.URL() + "/file.tar.gz",
					Revision:       "master/b8e362c206e3d0cbb7ed22ced771a0056455a2fb",
					Checksum:       "80ddfd18eb96f7d31cadc1a8a5171c6e2d95df3f6c23b0ed9cd8dddf6dba1406",
					LastUpdateTime: metav1.Time{Time: updatedTime},
				},
			}
			Expect(k8sClient.Status().Update(ctx, &testRepo)).Should(Succeed())

			By("checking that the status and its URL gets reconciled")
			gitRepoKey := types.NamespacedName{Namespace: "flux-system", Name: sourceName}
			createdRepo := sourcev1.GitRepository{}
			Expect(k8sClient.Get(ctx, gitRepoKey, &createdRepo)).Should(Succeed())

			By("creating a new TF and attaching to the repo")
			helloWorldTF := infrav1.Terraform{
				TypeMeta: metav1.TypeMeta{
					APIVersion: "infra.contrib.fluxcd.io/v1alpha1",
					Kind:       "Terraform",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name:      terraformName,
					Namespace: "flux-system",
				},
				Spec: infrav1.TerraformSpec{
					ApprovePlan: "auto",
					BackendConfig: &infrav1.BackendConfigSpec{
						SecretSuffix:    terraformName,
						InClusterConfig: false,
						ConfigPath:      testEnvKubeConfigPath,
					},
					Path: "./terraform-hello-world-example",
					SourceRef: infrav1.CrossNamespaceSourceReference{
						Kind:      "GitRepository",
						Name:      sourceName,
						Namespace: "flux-system",
					},
				},
			}
			Expect(k8sClient.Create(ctx, &helloWorldTF)).Should(Succeed())

			helloWorldTFKey := types.NamespacedName{Namespace: "flux-system", Name: terraformName}
			createdHelloWorldTF := infrav1.Terraform{}
			By("checking that the hello world TF get created")
			Eventually(func() bool {
				err := k8sClient.Get(ctx, helloWorldTFKey, &createdHelloWorldTF)
				if err != nil {
					return false
				}
				return true
			}, timeout, interval).Should(BeTrue())

			By("checking that the hello world TF get created")
			Eventually(func() int {
				err := k8sClient.Get(ctx, helloWorldTFKey, &createdHelloWorldTF)
				if err != nil {
					return -1
				}
				return len(createdHelloWorldTF.Status.Conditions)
			}, timeout, interval).ShouldNot(BeZero())

			By("checking that the applied status of the TF program is Applied, Successfully")
			Eventually(func() map[string]interface{} {
				err := k8sClient.Get(ctx, helloWorldTFKey, &createdHelloWorldTF)
				if err != nil {
					return nil
				}
				for _, c := range createdHelloWorldTF.Status.Conditions {
					if c.Type == "Apply" {
						return map[string]interface{}{
							"Type":   c.Type,
							"Reason": c.Reason,
						}
					}
				}
				return nil
			}, timeout, interval).Should(Equal(map[string]interface{}{
				"Type":   "Apply",
				"Reason": "TerraformAppliedSucceed",
			}))

			By("checking output condition")
			Eventually(func() map[string]interface{} {
				err := k8sClient.Get(ctx, helloWorldTFKey, &createdHelloWorldTF)
				if err != nil {
					return nil
				}
				for _, c := range createdHelloWorldTF.Status.Conditions {
					if c.Type == "Output" {
						return map[string]interface{}{
							"Type":   c.Type,
							"Reason": c.Reason,
						}
					}
				}
				return nil
			}, timeout, interval).Should(Equal(map[string]interface{}{
				"Type":   "Output",
				"Reason": "TerraformOutputAvailable",
			}))

			By("checking that we have outputs available in the TF object")
			Eventually(func() []string {
				err := k8sClient.Get(ctx, helloWorldTFKey, &createdHelloWorldTF)
				if err != nil {
					return nil
				}
				return createdHelloWorldTF.Status.AvailableOutputs
			}, timeout, interval).Should(Equal([]string{"hello_world"}))

			tfStateKey := types.NamespacedName{Namespace: "flux-system", Name: "tfstate-default-" + terraformName}
			tfStateSecret := corev1.Secret{}
			By("checking that we have state secret in the TF object's namespace")
			Eventually(func() string {
				err := k8sClient.Get(ctx, tfStateKey, &tfStateSecret)
				if err != nil {
					return ""
				}
				return tfStateSecret.Name
			}, timeout, interval).Should(Equal("tfstate-default-" + terraformName))
		})
	})
})
