//go:build e2e
// +build e2e

/*
Copyright 2026.

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

package e2e

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/xrizur/kubeEphemerals/test/utils"
)

// namespace where the project is deployed in
const namespace = "ephemeral-operator-system"

// serviceAccountName created for the project
const serviceAccountName = "ephemeral-operator-controller-manager"

// metricsServiceName is the name of the metrics service of the project
const metricsServiceName = "ephemeral-operator-controller-manager-metrics-service"

// metricsRoleBindingName is the name of the RBAC that will be created to allow get the metrics data
const metricsRoleBindingName = "ephemeral-operator-metrics-binding"

var _ = Describe("Manager", Ordered, func() {
	var controllerPodName string

	// Before running the tests, set up the environment by creating the namespace,
	// enforce the restricted security policy to the namespace, installing CRDs,
	// and deploying the controller.
	BeforeAll(func() {
		By("creating manager namespace")
		cmd := exec.Command("kubectl", "create", "ns", namespace)
		_, err := utils.Run(cmd)
		Expect(err).NotTo(HaveOccurred(), "Failed to create namespace")

		By("labeling the namespace to enforce the restricted security policy")
		cmd = exec.Command("kubectl", "label", "--overwrite", "ns", namespace,
			"pod-security.kubernetes.io/enforce=restricted")
		_, err = utils.Run(cmd)
		Expect(err).NotTo(HaveOccurred(), "Failed to label namespace with restricted policy")

		By("installing CRDs")
		cmd = exec.Command("make", "install")
		_, err = utils.Run(cmd)
		Expect(err).NotTo(HaveOccurred(), "Failed to install CRDs")

		By("deploying the controller-manager")
		cmd = exec.Command("make", "deploy", fmt.Sprintf("IMG=%s", managerImage))
		_, err = utils.Run(cmd)
		Expect(err).NotTo(HaveOccurred(), "Failed to deploy the controller-manager")

		By("patching deployment to enable UNSAFE_DEV_MODE for UI (e2e accepts requests without X-Forwarded-User)")
		cmd = exec.Command("kubectl", "set", "env", "deployment/ephemeral-operator-controller-manager",
			"UNSAFE_DEV_MODE=true", "-n", namespace, "--overwrite")
		_, err = utils.Run(cmd)
		Expect(err).NotTo(HaveOccurred(), "Failed to set UNSAFE_DEV_MODE on deployment")
		By("waiting for deployment rollout after env patch")
		cmd = exec.Command("kubectl", "rollout", "status", "deployment/ephemeral-operator-controller-manager",
			"-n", namespace, "--timeout=120s")
		_, err = utils.Run(cmd)
		Expect(err).NotTo(HaveOccurred(), "Deployment rollout after patch failed")
	})

	// After all tests have been executed, clean up by undeploying the controller, uninstalling CRDs,
	// and deleting the namespace.
	AfterAll(func() {
		By("cleaning up the curl pod for metrics")
		cmd := exec.Command("kubectl", "delete", "pod", "curl-metrics", "-n", namespace)
		_, _ = utils.Run(cmd)

		By("undeploying the controller-manager")
		cmd = exec.Command("make", "undeploy")
		_, _ = utils.Run(cmd)

		By("uninstalling CRDs")
		cmd = exec.Command("make", "uninstall")
		_, _ = utils.Run(cmd)

		By("removing manager namespace")
		cmd = exec.Command("kubectl", "delete", "ns", namespace)
		_, _ = utils.Run(cmd)
	})

	// After each test, check for failures and collect logs, events,
	// and pod descriptions for debugging.
	AfterEach(func() {
		specReport := CurrentSpecReport()
		if specReport.Failed() {
			By("Fetching controller manager pod logs")
			cmd := exec.Command("kubectl", "logs", controllerPodName, "-n", namespace)
			controllerLogs, err := utils.Run(cmd)
			if err == nil {
				_, _ = fmt.Fprintf(GinkgoWriter, "Controller logs:\n %s", controllerLogs)
			} else {
				_, _ = fmt.Fprintf(GinkgoWriter, "Failed to get Controller logs: %s", err)
			}

			By("Fetching Kubernetes events")
			cmd = exec.Command("kubectl", "get", "events", "-n", namespace, "--sort-by=.lastTimestamp")
			eventsOutput, err := utils.Run(cmd)
			if err == nil {
				_, _ = fmt.Fprintf(GinkgoWriter, "Kubernetes events:\n%s", eventsOutput)
			} else {
				_, _ = fmt.Fprintf(GinkgoWriter, "Failed to get Kubernetes events: %s", err)
			}

			By("Fetching curl-metrics logs")
			cmd = exec.Command("kubectl", "logs", "curl-metrics", "-n", namespace)
			metricsOutput, err := utils.Run(cmd)
			if err == nil {
				_, _ = fmt.Fprintf(GinkgoWriter, "Metrics logs:\n %s", metricsOutput)
			} else {
				_, _ = fmt.Fprintf(GinkgoWriter, "Failed to get curl-metrics logs: %s", err)
			}

			By("Fetching controller manager pod description")
			cmd = exec.Command("kubectl", "describe", "pod", controllerPodName, "-n", namespace)
			podDescription, err := utils.Run(cmd)
			if err == nil {
				fmt.Println("Pod description:\n", podDescription)
			} else {
				fmt.Println("Failed to describe controller pod")
			}
		}
	})

	SetDefaultEventuallyTimeout(2 * time.Minute)
	SetDefaultEventuallyPollingInterval(time.Second)

	Context("Manager", func() {
		It("should run successfully", func() {
			By("validating that the controller-manager pod is running as expected")
			verifyControllerUp := func(g Gomega) {
				// Get the name of the controller-manager pod
				cmd := exec.Command("kubectl", "get",
					"pods", "-l", "control-plane=controller-manager",
					"-o", "go-template={{ range .items }}"+
						"{{ if not .metadata.deletionTimestamp }}"+
						"{{ .metadata.name }}"+
						"{{ \"\\n\" }}{{ end }}{{ end }}",
					"-n", namespace,
				)

				podOutput, err := utils.Run(cmd)
				g.Expect(err).NotTo(HaveOccurred(), "Failed to retrieve controller-manager pod information")
				podNames := utils.GetNonEmptyLines(podOutput)
				g.Expect(podNames).To(HaveLen(1), "expected 1 controller pod running")
				controllerPodName = podNames[0]
				g.Expect(controllerPodName).To(ContainSubstring("controller-manager"))

				// Validate the pod's status
				cmd = exec.Command("kubectl", "get",
					"pods", controllerPodName, "-o", "jsonpath={.status.phase}",
					"-n", namespace,
				)
				output, err := utils.Run(cmd)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(output).To(Equal("Running"), "Incorrect controller-manager pod status")
			}
			Eventually(verifyControllerUp).Should(Succeed())
		})

		It("should ensure the metrics endpoint is serving metrics", func() {
			By("creating a ClusterRoleBinding for the service account to allow access to metrics")
			cmd := exec.Command("kubectl", "create", "clusterrolebinding", metricsRoleBindingName,
				"--clusterrole=ephemeral-operator-metrics-reader",
				fmt.Sprintf("--serviceaccount=%s:%s", namespace, serviceAccountName),
			)
			_, err := utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred(), "Failed to create ClusterRoleBinding")

			By("validating that the metrics service is available")
			cmd = exec.Command("kubectl", "get", "service", metricsServiceName, "-n", namespace)
			_, err = utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred(), "Metrics service should exist")

			By("getting the service account token")
			token, err := serviceAccountToken()
			Expect(err).NotTo(HaveOccurred())
			Expect(token).NotTo(BeEmpty())

			By("ensuring the controller pod is ready")
			verifyControllerPodReady := func(g Gomega) {
				cmd := exec.Command("kubectl", "get", "pod", controllerPodName, "-n", namespace,
					"-o", "jsonpath={.status.conditions[?(@.type=='Ready')].status}")
				output, err := utils.Run(cmd)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(output).To(Equal("True"), "Controller pod not ready")
			}
			Eventually(verifyControllerPodReady, 3*time.Minute, time.Second).Should(Succeed())

			By("verifying that the controller manager is serving the metrics server")
			verifyMetricsServerStarted := func(g Gomega) {
				cmd := exec.Command("kubectl", "logs", controllerPodName, "-n", namespace)
				output, err := utils.Run(cmd)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(output).To(ContainSubstring("Serving metrics server"),
					"Metrics server not yet started")
			}
			Eventually(verifyMetricsServerStarted, 3*time.Minute, time.Second).Should(Succeed())

			// +kubebuilder:scaffold:e2e-metrics-webhooks-readiness

			By("creating the curl-metrics pod to access the metrics endpoint")
			cmd = exec.Command("kubectl", "run", "curl-metrics", "--restart=Never",
				"--namespace", namespace,
				"--image=curlimages/curl:latest",
				"--overrides",
				fmt.Sprintf(`{
					"spec": {
						"containers": [{
							"name": "curl",
							"image": "curlimages/curl:latest",
							"command": ["/bin/sh", "-c"],
							"args": ["curl -v -k -H 'Authorization: Bearer %s' https://%s.%s.svc.cluster.local:8443/metrics"],
							"securityContext": {
								"readOnlyRootFilesystem": true,
								"allowPrivilegeEscalation": false,
								"capabilities": {
									"drop": ["ALL"]
								},
								"runAsNonRoot": true,
								"runAsUser": 1000,
								"seccompProfile": {
									"type": "RuntimeDefault"
								}
							}
						}],
						"serviceAccountName": "%s"
					}
				}`, token, metricsServiceName, namespace, serviceAccountName))
			_, err = utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred(), "Failed to create curl-metrics pod")

			By("waiting for the curl-metrics pod to complete.")
			verifyCurlUp := func(g Gomega) {
				cmd := exec.Command("kubectl", "get", "pods", "curl-metrics",
					"-o", "jsonpath={.status.phase}",
					"-n", namespace)
				output, err := utils.Run(cmd)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(output).To(Equal("Succeeded"), "curl pod in wrong status")
			}
			Eventually(verifyCurlUp, 5*time.Minute).Should(Succeed())

			By("getting the metrics by checking curl-metrics logs")
			verifyMetricsAvailable := func(g Gomega) {
				metricsOutput, err := getMetricsOutput()
				g.Expect(err).NotTo(HaveOccurred(), "Failed to retrieve logs from curl pod")
				g.Expect(metricsOutput).NotTo(BeEmpty())
				g.Expect(metricsOutput).To(ContainSubstring("< HTTP/1.1 200 OK"))
			}
			Eventually(verifyMetricsAvailable, 2*time.Minute).Should(Succeed())
		})

		It("should serve kubeconfig for environment (self-service kubeconfig)", func() {
			const envName = "e2e-kubeconfig"
			const envNamespace = "env-e2e-kubeconfig"
			const uiPort = "8082"

			By("ensuring controller pod name is set")
			if controllerPodName == "" {
				cmd := exec.Command("kubectl", "get", "pods", "-l", "control-plane=controller-manager",
					"-n", namespace, "-o", "jsonpath={.items[0].metadata.name}")
				podName, err := utils.Run(cmd)
				Expect(err).NotTo(HaveOccurred())
				controllerPodName = strings.TrimSpace(podName)
				Expect(controllerPodName).NotTo(BeEmpty())
			}

			By("getting project dir for fixture path")
			projectDir, err := utils.GetProjectDir()
			Expect(err).NotTo(HaveOccurred())
			fixturePath := filepath.Join(projectDir, "test", "e2e", "fixtures", "ephemeralenv_kubeconfig_e2e.yaml")

			By("creating EphemeralEnv for kubeconfig test")
			cmd := exec.Command("kubectl", "apply", "-f", fixturePath)
			_, err = utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred(), "Failed to apply EphemeralEnv fixture")

			defer func() {
				By("deleting EphemeralEnv after kubeconfig test")
				cmd := exec.Command("kubectl", "delete", "-f", fixturePath, "--ignore-not-found", "--wait=false")
				_, _ = utils.Run(cmd)
			}()

			By("waiting for env namespace to exist")
			Eventually(func() error {
				cmd := exec.Command("kubectl", "get", "namespace", envNamespace)
				_, err := utils.Run(cmd)
				return err
			}, 2*time.Minute, time.Second).Should(Succeed())

			By("waiting for developer-access ServiceAccount in env namespace")
			Eventually(func() error {
				cmd := exec.Command("kubectl", "get", "serviceaccount", "developer-access", "-n", envNamespace)
				_, err := utils.Run(cmd)
				return err
			}, 1*time.Minute, time.Second).Should(Succeed())

			By("starting port-forward to controller UI (8082)")
			portForwardCmd := exec.Command("kubectl", "port-forward", "pod/"+controllerPodName, uiPort+":"+uiPort, "-n", namespace)
			err = portForwardCmd.Start()
			Expect(err).NotTo(HaveOccurred(), "Failed to start port-forward")
			defer func() {
				if portForwardCmd.Process != nil {
					_ = portForwardCmd.Process.Kill()
				}
			}()

			By("waiting for UI port to be reachable")
			Eventually(func() error {
				resp, err := http.Get("http://127.0.0.1:" + uiPort + "/api/envs")
				if err != nil {
					return err
				}
				_ = resp.Body.Close()
				return nil
			}, 30*time.Second, time.Second).Should(Succeed())

			By("requesting kubeconfig for environment")
			var kubeconfigResp *http.Response
			Eventually(func() error {
				kubeconfigResp, err = http.Get("http://127.0.0.1:" + uiPort + "/api/envs/" + envName + "/kubeconfig")
				if err != nil {
					return err
				}
				if kubeconfigResp.StatusCode != http.StatusOK {
					_ = kubeconfigResp.Body.Close()
					return fmt.Errorf("unexpected status %d", kubeconfigResp.StatusCode)
				}
				return nil
			}, 30*time.Second, time.Second).Should(Succeed())
			Expect(kubeconfigResp).NotTo(BeNil())
			defer kubeconfigResp.Body.Close()

			body, err := io.ReadAll(kubeconfigResp.Body)
			Expect(err).NotTo(HaveOccurred())

			bodyStr := string(body)
			Expect(bodyStr).To(ContainSubstring(envNamespace), "kubeconfig should contain env namespace")
			Expect(bodyStr).To(ContainSubstring("clusters:"), "kubeconfig should contain clusters")
			Expect(bodyStr).To(ContainSubstring("users:"), "kubeconfig should contain users")
			Expect(bodyStr).To(ContainSubstring("contexts:"), "kubeconfig should contain contexts")
			Expect(bodyStr).To(ContainSubstring("current-context:"), "kubeconfig should contain current-context")
			Expect(kubeconfigResp.Header.Get("Content-Disposition")).To(ContainSubstring("kubeconfig-"+envName), "response should suggest kubeconfig filename")
		})

		// +kubebuilder:scaffold:e2e-webhooks-checks

		Context("EphemeralEnv lifecycle and corner cases", func() {
			const uiPortCorner = "8082"
			const envNameAlice = "e2e-corner-alice"
			const envNamespaceAlice = "env-e2e-corner-alice"

			It("should create EphemeralEnv with owner and set namespace/status", func() {
				projectDir, err := utils.GetProjectDir()
				Expect(err).NotTo(HaveOccurred())
				fixturePath := filepath.Join(projectDir, "test", "e2e", "fixtures", "ephemeralenv_corner_alice.yaml")
				cmd := exec.Command("kubectl", "apply", "-f", fixturePath)
				_, err = utils.Run(cmd)
				Expect(err).NotTo(HaveOccurred())
				defer func() {
					exec.Command("kubectl", "delete", "-f", fixturePath, "--ignore-not-found", "--wait=false").Run()
				}()

				Eventually(func() error {
					cmd := exec.Command("kubectl", "get", "namespace", envNamespaceAlice)
					_, err := utils.Run(cmd)
					return err
				}, 2*time.Minute, time.Second).Should(Succeed())

				Eventually(func() error {
					cmd := exec.Command("kubectl", "get", "ephemeralenv", envNameAlice, "-n", "default",
						"-o", "jsonpath={.status.activeNamespace}")
					out, err := utils.Run(cmd)
					if err != nil {
						return err
					}
					if strings.TrimSpace(out) != envNamespaceAlice {
						return fmt.Errorf("activeNamespace not set: %q", out)
					}
					return nil
				}).Should(Succeed())
			})

			It("should list envs for alice when X-Forwarded-User is alice", func() {
				portForwardCmd := exec.Command("kubectl", "port-forward", "pod/"+controllerPodName, uiPortCorner+":"+uiPortCorner, "-n", namespace)
				Expect(portForwardCmd.Start()).To(Succeed())
				defer func() {
					if portForwardCmd.Process != nil {
						_ = portForwardCmd.Process.Kill()
					}
				}()

				Eventually(func() error {
					resp, err := http.Get("http://127.0.0.1:" + uiPortCorner + "/api/envs")
					if err != nil {
						return err
					}
					_ = resp.Body.Close()
					return nil
				}, 15*time.Second, time.Second).Should(Succeed())

				req, err := http.NewRequest(http.MethodGet, "http://127.0.0.1:"+uiPortCorner+"/api/envs", nil)
				Expect(err).NotTo(HaveOccurred())
				req.Header.Set("X-Forwarded-User", "alice")
				resp, err := http.DefaultClient.Do(req)
				Expect(err).NotTo(HaveOccurred())
				defer resp.Body.Close()
				Expect(resp.StatusCode).To(Equal(http.StatusOK))
				body, err := io.ReadAll(resp.Body)
				Expect(err).NotTo(HaveOccurred())
				var list []map[string]interface{}
				Expect(json.Unmarshal(body, &list)).To(Succeed())
				var found bool
				for _, e := range list {
					meta, _ := e["metadata"].(map[string]interface{})
					if name, _ := meta["name"].(string); name == envNameAlice {
						found = true
						break
					}
				}
				Expect(found).To(BeTrue(), "alice's env should be in list")
			})

			It("should return 403 when bob deletes alice's env", func() {
				portForwardCmd := exec.Command("kubectl", "port-forward", "pod/"+controllerPodName, uiPortCorner+":"+uiPortCorner, "-n", namespace)
				Expect(portForwardCmd.Start()).To(Succeed())
				defer func() {
					if portForwardCmd.Process != nil {
						_ = portForwardCmd.Process.Kill()
					}
				}()
				Eventually(func() error {
					resp, err := http.Get("http://127.0.0.1:" + uiPortCorner + "/api/envs")
					if err != nil {
						return err
					}
					_ = resp.Body.Close()
					return nil
				}, 15*time.Second, time.Second).Should(Succeed())

				req, err := http.NewRequest(http.MethodDelete, "http://127.0.0.1:"+uiPortCorner+"/api/envs/"+envNameAlice, nil)
				Expect(err).NotTo(HaveOccurred())
				req.Header.Set("X-Forwarded-User", "bob")
				resp, err := http.DefaultClient.Do(req)
				Expect(err).NotTo(HaveOccurred())
				defer resp.Body.Close()
				Expect(resp.StatusCode).To(Equal(http.StatusForbidden))
			})

			It("should return 204 when alice deletes own env", func() {
				portForwardCmd := exec.Command("kubectl", "port-forward", "pod/"+controllerPodName, uiPortCorner+":"+uiPortCorner, "-n", namespace)
				Expect(portForwardCmd.Start()).To(Succeed())
				defer func() {
					if portForwardCmd.Process != nil {
						_ = portForwardCmd.Process.Kill()
					}
				}()
				Eventually(func() error {
					resp, err := http.Get("http://127.0.0.1:" + uiPortCorner + "/api/envs")
					if err != nil {
						return err
					}
					_ = resp.Body.Close()
					return nil
				}, 15*time.Second, time.Second).Should(Succeed())

				req, err := http.NewRequest(http.MethodDelete, "http://127.0.0.1:"+uiPortCorner+"/api/envs/"+envNameAlice, nil)
				Expect(err).NotTo(HaveOccurred())
				req.Header.Set("X-Forwarded-User", "alice")
				resp, err := http.DefaultClient.Do(req)
				Expect(err).NotTo(HaveOccurred())
				defer resp.Body.Close()
				Expect(resp.StatusCode).To(Equal(http.StatusNoContent))
			})

			It("should list templates via GET /api/templates", func() {
				portForwardCmd := exec.Command("kubectl", "port-forward", "pod/"+controllerPodName, uiPortCorner+":"+uiPortCorner, "-n", namespace)
				Expect(portForwardCmd.Start()).To(Succeed())
				defer func() {
					if portForwardCmd.Process != nil {
						_ = portForwardCmd.Process.Kill()
					}
				}()
				Eventually(func() error {
					resp, err := http.Get("http://127.0.0.1:" + uiPortCorner + "/api/templates")
					if err != nil {
						return err
					}
					_ = resp.Body.Close()
					return nil
				}, 15*time.Second, time.Second).Should(Succeed())

				resp, err := http.Get("http://127.0.0.1:" + uiPortCorner + "/api/templates")
				Expect(err).NotTo(HaveOccurred())
				defer resp.Body.Close()
				Expect(resp.StatusCode).To(Equal(http.StatusOK))
			})

			It("should handle invalid EphemeralEnv (missing gateway namespace)", func() {
				projectDir, err := utils.GetProjectDir()
				Expect(err).NotTo(HaveOccurred())
				invalidPath := filepath.Join(projectDir, "test", "e2e", "fixtures", "ephemeralenv_invalid_gateway.yaml")
				cmd := exec.Command("kubectl", "apply", "-f", invalidPath)
				out, err := utils.Run(cmd)
				if err != nil {
					Expect(out).To(Or(ContainSubstring("error"), ContainSubstring("invalid")))
					return
				}
				defer func() {
					exec.Command("kubectl", "delete", "-f", invalidPath, "--ignore-not-found", "--wait=false").Run()
				}()
				Eventually(func() string {
					cmd := exec.Command("kubectl", "get", "ephemeralenv", "e2e-invalid-gw", "-n", "default",
						"-o", "jsonpath={.status.phase}")
					o, _ := utils.Run(cmd)
					return strings.TrimSpace(o)
				}, 30*time.Second).Should(Or(Equal("Pending"), Equal("Failed"), BeEmpty()))
			})
		})

		// TODO: Customize the e2e test suite with scenarios specific to your project.
		// Consider applying sample/CR(s) and check their status and/or verifying
		// the reconciliation by using the metrics, i.e.:
		// metricsOutput, err := getMetricsOutput()
		// Expect(err).NotTo(HaveOccurred(), "Failed to retrieve logs from curl pod")
		// Expect(metricsOutput).To(ContainSubstring(
		//    fmt.Sprintf(`controller_runtime_reconcile_total{controller="%s",result="success"} 1`,
		//    strings.ToLower(<Kind>),
		// ))
	})
})

// HelmChartNamespace is the namespace used when installing via Helm in e2e.
// Use a different namespace than Manager (ephemeral-operator-system) to avoid
// conflicts when both suites run (e.g. namespace Terminating).
const helmChartNamespace = "ephemeral-operator-helm-system"

// HelmReleaseName is the Helm release name used in e2e.
const helmReleaseName = "ephemeral-operator"

var _ = Describe("Helm chart", Ordered, func() {
	// Helm chart e2e: install via Helm (after Manager/Kustomize tests have run and cleaned up),
	// then verify pod, CRDs, and that CRs can be created.
	BeforeAll(func() {
		By("getting project dir for chart path")
		projectDir, err := utils.GetProjectDir()
		Expect(err).NotTo(HaveOccurred())
		chartPath := filepath.Join(projectDir, "charts", "ephemeral-operator")
		info, err := os.Stat(chartPath)
		Expect(err).NotTo(HaveOccurred(), "chart path should exist")
		Expect(info.IsDir()).To(BeTrue(), "chart path should be a directory")

		// Parse managerImage (e.g. "example.com/ephemeral-operator:v0.0.1") into repo and tag
		lastColon := strings.LastIndex(managerImage, ":")
		imageRepo := managerImage
		imageTag := "latest"
		if lastColon > 0 {
			imageRepo = managerImage[:lastColon]
			imageTag = managerImage[lastColon+1:]
		}

		By("installing operator via Helm chart")
		cmd := exec.Command("helm", "upgrade", "--install", helmReleaseName, chartPath,
			"--namespace", helmChartNamespace,
			"--create-namespace",
			"--set", "image.repository="+imageRepo,
			"--set", "image.tag="+imageTag,
			"--set", "image.pullPolicy=IfNotPresent",
			"--set", "config.metricsBindAddress=0", // disable metrics for simpler e2e
		)
		_, err = utils.Run(cmd)
		Expect(err).NotTo(HaveOccurred(), "Failed to install Helm chart")
	})

	AfterAll(func() {
		By("uninstalling Helm release")
		cmd := exec.Command("helm", "uninstall", helmReleaseName, "--namespace", helmChartNamespace)
		_, _ = utils.Run(cmd)
		By("removing Helm chart namespace")
		cmd = exec.Command("kubectl", "delete", "ns", helmChartNamespace, "--ignore-not-found", "--wait=false")
		_, _ = utils.Run(cmd)
	})

	SetDefaultEventuallyTimeout(2 * time.Minute)
	SetDefaultEventuallyPollingInterval(time.Second)

	Context("Helm release", func() {
		It("should install and run the operator pod", func() {
			By("waiting for operator pod to be Running")
			Eventually(func() error {
				cmd := exec.Command("kubectl", "get", "pods", "-l", "app.kubernetes.io/name=ephemeral-operator",
					"-n", helmChartNamespace, "-o", "jsonpath={.items[0].status.phase}")
				out, err := utils.Run(cmd)
				if err != nil {
					return err
				}
				if strings.TrimSpace(out) != "Running" {
					return fmt.Errorf("pod phase is %q", out)
				}
				return nil
			}).Should(Succeed())
		})

		It("should have CRDs installed", func() {
			By("checking EphemeralEnv CRD exists")
			cmd := exec.Command("kubectl", "get", "crd", "ephemeralenvs.ephemeral.ephemeralenv.io")
			_, err := utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred(), "EphemeralEnv CRD should exist")

			By("checking EnvironmentTemplate CRD exists")
			cmd = exec.Command("kubectl", "get", "crd", "environmenttemplates.ephemeral.ephemeralenv.io")
			_, err = utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred(), "EnvironmentTemplate CRD should exist")
		})

		It("should reconcile EnvironmentTemplate when created", func() {
			projectDir, err := utils.GetProjectDir()
			Expect(err).NotTo(HaveOccurred())
			samplePath := filepath.Join(projectDir, "config", "samples", "ephemeral_v1alpha1_environmenttemplate.yaml")
			// Sample file references namespace ephemeral-system; ensure it exists
			By("creating namespace ephemeral-system for sample")
			cmd := exec.Command("kubectl", "create", "namespace", "ephemeral-system")
			_, _ = utils.Run(cmd) // ignore error if already exists
			By("applying EnvironmentTemplate sample")
			cmd = exec.Command("kubectl", "apply", "-f", samplePath)
			_, err = utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred(), "Failed to apply EnvironmentTemplate sample")
			defer func() {
				cmd := exec.Command("kubectl", "delete", "-f", samplePath, "--ignore-not-found", "--wait=false")
				_, _ = utils.Run(cmd)
			}()

			By("waiting for EnvironmentTemplate to be listed")
			Eventually(func() error {
				cmd := exec.Command("kubectl", "get", "environmenttemplates.ephemeral.ephemeralenv.io", "-A", "--no-headers")
				out, err := utils.Run(cmd)
				if err != nil {
					return err
				}
				// kubectl --no-headers returns "namespace   name   displayName"; no literal "environmenttemplate"
				if strings.TrimSpace(out) == "" {
					return fmt.Errorf("no EnvironmentTemplate listed yet (empty output)")
				}
				return nil
			}).Should(Succeed())
		})
	})
})

// serviceAccountToken returns a token for the specified service account in the given namespace.
// It uses the Kubernetes TokenRequest API to generate a token by directly sending a request
// and parsing the resulting token from the API response.
func serviceAccountToken() (string, error) {
	const tokenRequestRawString = `{
		"apiVersion": "authentication.k8s.io/v1",
		"kind": "TokenRequest"
	}`

	// Temporary file to store the token request
	secretName := fmt.Sprintf("%s-token-request", serviceAccountName)
	tokenRequestFile := filepath.Join("/tmp", secretName)
	err := os.WriteFile(tokenRequestFile, []byte(tokenRequestRawString), os.FileMode(0o644))
	if err != nil {
		return "", err
	}

	var out string
	verifyTokenCreation := func(g Gomega) {
		// Execute kubectl command to create the token
		cmd := exec.Command("kubectl", "create", "--raw", fmt.Sprintf(
			"/api/v1/namespaces/%s/serviceaccounts/%s/token",
			namespace,
			serviceAccountName,
		), "-f", tokenRequestFile)

		output, err := cmd.CombinedOutput()
		g.Expect(err).NotTo(HaveOccurred())

		// Parse the JSON output to extract the token
		var token tokenRequest
		err = json.Unmarshal(output, &token)
		g.Expect(err).NotTo(HaveOccurred())

		out = token.Status.Token
	}
	Eventually(verifyTokenCreation).Should(Succeed())

	return out, err
}

// getMetricsOutput retrieves and returns the logs from the curl pod used to access the metrics endpoint.
func getMetricsOutput() (string, error) {
	By("getting the curl-metrics logs")
	cmd := exec.Command("kubectl", "logs", "curl-metrics", "-n", namespace)
	return utils.Run(cmd)
}

// tokenRequest is a simplified representation of the Kubernetes TokenRequest API response,
// containing only the token field that we need to extract.
type tokenRequest struct {
	Status struct {
		Token string `json:"token"`
	} `json:"status"`
}
