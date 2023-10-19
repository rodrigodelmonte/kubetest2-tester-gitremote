package tester

import (
	"flag"
	"fmt"
	"os"
	"strconv"
	"time"

	git "github.com/go-git/go-git/v5"
	"github.com/kballard/go-shellquote"
	"github.com/octago/sflags/gen/gpflag"
	"k8s.io/klog"
	"sigs.k8s.io/kubetest2/pkg/artifacts"
	"sigs.k8s.io/kubetest2/pkg/exec"
	"sigs.k8s.io/kubetest2/pkg/testers"
)

var GitTag string

type Tester struct {
	FlakeAttempts int           `desc:"Make up to this many attempts to run each spec."`
	GinkgoArgs    string        `desc:"Additional arguments supported by the ginkgo binary."`
	Parallel      int           `desc:"Run this many tests in parallel at once."`
	SkipRegex     string        `desc:"Regular expression of jobs to skip."`
	FocusRegex    string        `desc:"Regular expression of jobs to focus on."`
	Timeout       time.Duration `desc:"How long (in golang duration format) to wait for ginkgo tests to complete."`
	Env           []string      `desc:"List of env variables to pass to ginkgo libraries"`
	Repo          string        `desc:"Git repo to clone for the test."`

	kubeconfigPath string
	runDir         string

	// These paths are set up by AcquireTestPackage()
	e2eTestPath string
	ginkgoPath  string
	kubectlPath string
}

func (t *Tester) Execute() error {
	fs, err := gpflag.Parse(t)
	if err != nil {
		return fmt.Errorf("failed to initialize tester: %v", err)
	}

	fs.AddGoFlagSet(flag.CommandLine)

	help := fs.BoolP("help", "h", false, "")

	if err := fs.Parse(os.Args); err != nil {
		return fmt.Errorf("failed to parse flags: %v", err)
	}

	if *help {
		fs.SetOutput(os.Stdout)
		fs.PrintDefaults()
		return nil
	}

	if err := t.initKubetest2Info(); err != nil {
		return err
	}
	return t.Test()
}

// initializes relevant information from the well defined kubetest2 environment variables.
func (t *Tester) initKubetest2Info() error {
	if dir, ok := os.LookupEnv("KUBETEST2_RUN_DIR"); ok {
		t.runDir = dir
		return nil
	}
	// default to current working directory if for some reason the env is not set
	dir, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("failed to set run dir: %v", err)
	}
	t.runDir = dir
	return nil
}

func (t *Tester) Test() error {

	if err := testers.WriteVersionToMetadata(GitTag); err != nil {
		return err
	}

	if err := t.pretestSetup(); err != nil {
		return err
	}

	if t.kubeconfigPath == "" {
		if kubeconfig, ok := os.LookupEnv("KUBECONFIG"); ok {
			t.kubeconfigPath = kubeconfig
		} else {
			return fmt.Errorf("kubeconfig path not provided")
		}
	}

	e2eTestArgs := []string{
		"--kubeconfig=" + t.kubeconfigPath,
		"--ginkgo.skip=" + t.SkipRegex,
		"--ginkgo.focus=" + t.FocusRegex,
		"--report-dir=" + artifacts.BaseDir(),
		"--ginkgo.timeout=" + t.Timeout.String(),
	}

	extraGingkoArgs, err := shellquote.Split(t.GinkgoArgs)
	if err != nil {
		return fmt.Errorf("error parsing --gingko-args: %v", err)
	}
	ginkgoArgs := append(extraGingkoArgs,
		"--nodes="+strconv.Itoa(t.Parallel),
		t.e2eTestPath,
		"--")
	ginkgoArgs = append(ginkgoArgs, e2eTestArgs...)

	klog.V(0).Infof("Running ginkgo test as %s %+v", t.ginkgoPath, ginkgoArgs)
	cmd := exec.Command(t.ginkgoPath, ginkgoArgs...)
	cmd.SetEnv(t.Env...)
	exec.InheritOutput(cmd)
	return cmd.Run()
}

func (t *Tester) pretestSetup() error {

	_, err := git.PlainClone(t.runDir, false, &git.CloneOptions{
		URL: t.Repo,
	})
	if err != nil {
		return fmt.Errorf("failed to clone repo: %v", err)
	}

	return nil
}

func NewDefaultTester() *Tester {

	return &Tester{
		FlakeAttempts: 1,
		Parallel:      1,
		Timeout:       24 * time.Hour,
		Env:           nil,
	}
}

func Main() {
	t := NewDefaultTester()
	if err := t.Execute(); err != nil {
		klog.Fatalf("failed to run ginkgo tester: %v", err)
	}
}
