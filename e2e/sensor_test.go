package e2e_test

import (
	"fmt"
	"os"
	"os/exec"
	"syscall"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// watchPathFindings is only for the commented findings context; the active
// no_findings suite uses paths from testdata/sensor-e2e.yaml (nil override).
const (
	watchPathNoFindings = "watchedDir/no_findings/"
	watchPathFindings   = "watchedDir/findings/"
)

var _ = Describe("Git Sensor", func() {

	Context("with clean resources (no_findings)", Ordered, func() {
		var (
			msrvURL  string
			sensURL  string
			msrvProc *exec.Cmd
			sensProc *exec.Cmd
			msrvDir  string
			cfgFile  string
		)

		BeforeAll(func() {
			By("no_findings: starting modelsrv (then waiting for /api/test)")
			var err error
			msrvDir, err = os.MkdirTemp("", "e2e-modelsrv-nf-*")
			Expect(err).NotTo(HaveOccurred())

			msrvAddr := fmt.Sprintf("127.0.0.1:%d", freePort())
			msrvURL = "http://" + msrvAddr
			msrvProc = startProcessInDir(msrvDir, logPath("modelsrv-nf"), modelsrvBin(),
				"server", "--service-addr", msrvAddr)

			Eventually(func() error { return checkReady(msrvURL + "/api/test") }).
				WithTimeout(20 * time.Second).WithPolling(300 * time.Millisecond).Should(Succeed())

			By("no_findings: starting sensor (then waiting for /api/test)")
			sensAddr := fmt.Sprintf("127.0.0.1:%d", freePort())
			sensURL = "http://" + sensAddr
			cfgFile = buildRuntimeSensorConfig(loadBaseConfig(), targetRepoDir, msrvURL+"/api/", []string{watchPathNoFindings})
			sensProc = startProcess(logPath("sensor-nf"), sensorBin(),
				"-config", cfgFile, "-listen", sensAddr, "-poll-interval", "2s")

			Eventually(func() error { return checkReady(sensURL + "/api/test") }).
				WithTimeout(20 * time.Second).WithPolling(300 * time.Millisecond).Should(Succeed())

			By("no_findings: brief settle before assertions")
			time.Sleep(3 * time.Second)
		})

		AfterAll(func() {
			killProc(sensProc)
			killProc(msrvProc)
			os.RemoveAll(msrvDir)
			os.Remove(cfgFile)
		})

		It("forwards resources to modelsrv", func() {
			By("no_findings: asserting landscape APIs populate")
			endpoints := []struct {
				label, path string
				minLen      int
			}{
				{"NodeTypes", "/api/landscape/nodeTypes", 1},
				{"ContextTypes", "/api/landscape/contextTypes", 1},
				{"Nodes", "/api/landscape/nodes", 1},
				{"Contexts", "/api/landscape/contexts", 1},
			}
			for _, ep := range endpoints {
				By(fmt.Sprintf("checking %s", ep.label))
				Eventually(func() (int, error) {
					list, err := httpGetList(msrvURL + ep.path)
					return len(list), err
				}).WithTimeout(15*time.Second).WithPolling(500*time.Millisecond).
					Should(BeNumerically(">=", ep.minLen), "%s count too low", ep.label)
			}
		})

		It("produces zero findings", func() {
			By("no_findings: expecting empty findings list")
			list, err := httpGetList(msrvURL + "/api/landscape/findings")
			Expect(err).NotTo(HaveOccurred())
			Expect(list).To(BeEmpty(), "clean resources should produce no findings")
		})
	})

	Context("with broken references (findings)", Ordered, func() {
		var (
			msrvURL  string
			sensURL  string
			msrvProc *exec.Cmd
			sensProc *exec.Cmd
			msrvDir  string
			cfgFile  string
		)

		BeforeAll(func() {
			By("findings: starting modelsrv (then waiting for /api/test)")
			var err error
			msrvDir, err = os.MkdirTemp("", "e2e-modelsrv-fd-*")
			Expect(err).NotTo(HaveOccurred())

			msrvAddr := fmt.Sprintf("127.0.0.1:%d", freePort())
			msrvURL = "http://" + msrvAddr
			msrvProc = startProcessInDir(msrvDir, logPath("modelsrv-fd"), modelsrvBin(),
				"server", "--service-addr", msrvAddr)

			Eventually(func() error { return checkReady(msrvURL + "/api/test") }).
				WithTimeout(20 * time.Second).WithPolling(300 * time.Millisecond).Should(Succeed())

			By("findings: starting sensor on broken-ref paths (then waiting for /api/test)")
			sensAddr := fmt.Sprintf("127.0.0.1:%d", freePort())
			sensURL = "http://" + sensAddr
			cfgFile = buildRuntimeSensorConfig(loadBaseConfig(), targetRepoDir, msrvURL+"/api/",
				[]string{watchPathFindings})
			sensProc = startProcess(logPath("sensor-fd"), sensorBin(),
				"-config", cfgFile, "-listen", sensAddr, "-poll-interval", "2s")

			Eventually(func() error { return checkReady(sensURL + "/api/test") }).
				WithTimeout(20 * time.Second).WithPolling(300 * time.Millisecond).Should(Succeed())

			By("findings: brief settle before assertions")
			time.Sleep(3 * time.Second)
		})

		AfterAll(func() {
			killProc(sensProc)
			killProc(msrvProc)
			os.RemoveAll(msrvDir)
			os.Remove(cfgFile)
		})

		It("forwards resources to modelsrv", func() {
			By("findings: asserting landscape APIs populate")
			endpoints := []struct {
				label, path string
				minLen      int
			}{
				{"ContextTypes", "/api/landscape/contextTypes", 1},
				{"Contexts", "/api/landscape/contexts", 1},
				{"Nodes", "/api/landscape/nodes", 1},
			}
			for _, ep := range endpoints {
				By(fmt.Sprintf("checking %s", ep.label))
				Eventually(func() (int, error) {
					list, err := httpGetList(msrvURL + ep.path)
					return len(list), err
				}).WithTimeout(15*time.Second).WithPolling(500*time.Millisecond).
					Should(BeNumerically(">=", ep.minLen), "%s count too low", ep.label)
			}
		})

		It("creates findings for broken references", func() {
			By("findings: waiting for non-empty findings list")
			Eventually(func() (int, error) {
				list, err := httpGetList(msrvURL + "/api/landscape/findings")
				return len(list), err
			}).WithTimeout(15*time.Second).WithPolling(500*time.Millisecond).
				Should(BeNumerically(">=", 2), "expected multiple findings from broken resources")
		})

		It("registers finding types", func() {
			By("findings: waiting for findingTypes")
			Eventually(func() (int, error) {
				list, err := httpGetList(msrvURL + "/api/landscape/findingTypes")
				return len(list), err
			}).WithTimeout(10*time.Second).WithPolling(500*time.Millisecond).
				Should(BeNumerically(">=", 1), "expected at least one finding type")
		})
	})

	It("emits delete events on shutdown", func() {
		// On SIGTERM, reconcile.Run exits, calls sensor.Server.Close(), which runs
		// DeleteNodeById for the embedded git-sensor node; that must replicate as
		// a Node delete to subscribers. Use a stub subscriber instead of modelsrv.
		By("shutdown: starting stub subscriber")
		collector := newEventCollector()
		defer collector.stop()

		Eventually(func() error { return checkReady(collector.URL() + "/test") }).
			WithTimeout(3*time.Second).WithPolling(50*time.Millisecond).Should(Succeed())

		By("shutdown: starting sensor against stub")
		sensPort := freePort()
		sensHost := fmt.Sprintf("127.0.0.1:%d", sensPort)
		sensURL := "http://" + sensHost
		subscriberBase := collector.URL() + "/"

		cfgFile := buildRuntimeSensorConfig(loadBaseConfig(), targetRepoDir, subscriberBase,
			[]string{"watchedDir/no_findings"})
		defer func() { _ = os.Remove(cfgFile) }()

		logFile := logPath("sensor-shutdown")
		sensProc := startProcess(logFile, sensorBin(),
			"-config", cfgFile, "-listen", sensHost, "-poll-interval", "5s")

		Eventually(func() error { return checkReady(sensURL + "/api/test") }).
			WithTimeout(20*time.Second).WithPolling(200*time.Millisecond).Should(Succeed())

		By("shutdown: SIGTERM sensor, then expect Node Delete on subscriber")
		Expect(sensProc.Process.Signal(syscall.SIGTERM)).To(Succeed())

		done := make(chan error, 1)
		go func() { done <- sensProc.Wait() }()
		select {
		case err := <-done:
			Expect(err).ToNot(HaveOccurred(), "sensor should exit cleanly on SIGTERM")
		case <-time.After(20 * time.Second):
			killProc(sensProc)
			Fail("sensor process did not exit after SIGTERM")
		}

		Eventually(func() bool {
			return collector.hasEvent("Node", "Delete")
		}).WithTimeout(8*time.Second).WithPolling(50*time.Millisecond).
			Should(BeTrue(), "subscriber should receive Node Delete when sensor shuts down (git-sensor node deregistration)")
	})
})
