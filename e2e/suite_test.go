package e2e_test

import (
	"fmt"
	"os"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestE2E(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Git Sensor E2E")
}

var targetRepoDir string

var _ = BeforeSuite(func() {
	baseCfg := loadBaseConfig()
	Expect(baseCfg.Repos).NotTo(BeEmpty(), "sensor-e2e.yaml must define at least one repo")
	repo := baseCfg.Repos[0]

	branch := repo.Branch
	if branch == "" {
		branch = "main"
	}

	By(fmt.Sprintf("cloning %s @ %s", repo.Repo, branch))
	var err error
	targetRepoDir, err = os.MkdirTemp("", "e2e-target-*")
	Expect(err).NotTo(HaveOccurred())
	Expect(os.RemoveAll(targetRepoDir)).To(Succeed())
	// Clone into a temp dir — not sensor-e2e.yaml checkoutDir (.work/...), which
	// is for local/manual runs with a remote repo URL.
	mustRun("git", "clone", "--depth=1", "--branch", branch, repo.Repo, targetRepoDir)
	By("fixture repo cloned; proceeding to specs")
})

var _ = AfterSuite(func() {
	if targetRepoDir != "" {
		os.RemoveAll(targetRepoDir)
	}
})
