/*
Copyright 2019 The Tekton Authors

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

package git

import (
	"bytes"
	"errors"
	"fmt"
	"math"
	"math/rand"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	homedir "github.com/mitchellh/go-homedir"

	"go.uber.org/zap"
)

const (
	sshKnownHostsUserPath          = ".ssh/known_hosts"
	sshMissingKnownHostsSSHCommand = "ssh -o StrictHostKeyChecking=accept-new"
)

// sshURLRegexFormat matches the url of SSH git repository
var sshURLRegexFormat = regexp.MustCompile(`(ssh://[\w\d\.]+|.+@?.+\..+:)(:[\d]+){0,1}/*(.*)`)

// GitError wraps a failed git command with its stderr/stdout output.
type GitError struct {
	Args   []string
	Dir    string
	Output string
	Err    error
}

func (e *GitError) Error() string {
	out := strings.TrimSpace(e.Output)
	if out != "" {
		return fmt.Sprintf("%v: %s", e.Err, out)
	}
	return fmt.Sprintf("%v", e.Err)
}

func (e *GitError) Unwrap() error { return e.Err }

func run(logger *zap.SugaredLogger, dir string, args ...string) (string, error) {
	c := exec.Command("git", args...)
	var output bytes.Buffer
	c.Stderr = &output
	c.Stdout = &output
	// This is the optional working directory. If not set, it defaults to the current
	// working directory of the process.
	if dir != "" {
		c.Dir = dir
	}
	if err := c.Run(); err != nil {
		logger.Errorf("Error running git %v: %v\n%v", args, err, output.String())
		return "", &GitError{Args: args, Dir: dir, Output: output.String(), Err: err}
	}
	return output.String(), nil
}

// FetchSpec describes how to initialize and fetch from a Git repository.
type FetchSpec struct {
	URL                       string
	Revision                  string
	Refspec                   string
	Path                      string
	Depth                     uint
	Submodules                bool
	SubmodulePaths            []string
	SSLVerify                 bool
	HTTPProxy                 string
	HTTPSProxy                string
	NOProxy                   string
	SparseCheckoutDirectories string
}

type RetryConfig struct {
	Initial     time.Duration
	Max         time.Duration
	Factor      float64
	MaxAttempts int
}

// Fetch fetches the specified git repository at the revision into path, using the refspec to fetch if provided.
func Fetch(logger *zap.SugaredLogger, spec FetchSpec, retryConfig RetryConfig) error {
	homepath, err := homedir.Dir()
	if err != nil {
		logger.Errorf("Unexpected error getting the user home directory: %v", err)
		return err
	}
	if os.Geteuid() == 0 {
		homepath = "/root"
	}
	ensureHomeEnv(logger, homepath)
	validateGitAuth(logger, "/tekton/creds", spec.URL)

	if spec.Path != "" {
		if _, err := run(logger, "", "init", spec.Path); err != nil {
			return err
		}
		if err := os.Chdir(spec.Path); err != nil {
			return fmt.Errorf("failed to change directory with path %s; err: %w", spec.Path, err)
		}
		if _, err := run(logger, "", "config", "--add", "--global", "safe.directory", spec.Path); err != nil {
			return err
		}
	} else {
		if _, err := run(logger, "", "init"); err != nil {
			return err
		}
		if _, err := run(logger, "", "config", "--add", "--global", "safe.directory", "/"); err != nil {
			return err
		}
	}
	if err := configSparseCheckout(logger, spec); err != nil {
		return err
	}
	trimmedURL := strings.TrimSpace(spec.URL)

	// Check existing remotes to decide whether to add or update origin
	remotes, err := run(logger, "", "remote")
	if err != nil {
		return fmt.Errorf("failed to list git remotes: %w", err)
	}

	// Check if "origin" remote already exists
	remoteExists := false
	for _, remote := range strings.Fields(remotes) {
		if strings.TrimSpace(remote) == "origin" {
			remoteExists = true
			break
		}
	}

	if remoteExists {
		// Remote exists, update its URL
		if _, err := run(logger, "", "remote", "set-url", "origin", trimmedURL); err != nil {
			return fmt.Errorf("failed to update origin remote URL: %w", err)
		}
	} else {
		// Remote doesn't exist, add it
		if _, err := run(logger, "", "remote", "add", "origin", trimmedURL); err != nil {
			return fmt.Errorf("failed to add origin remote: %w", err)
		}
	}

	hasKnownHosts, err := userHasKnownHostsFile(homepath)
	if err != nil {
		return fmt.Errorf("error checking for known_hosts file: %w", err)
	}
	if !hasKnownHosts {
		if _, err := run(logger, "", "config", "core.sshCommand", sshMissingKnownHostsSSHCommand); err != nil {
			err = fmt.Errorf("error disabling strict host key checking: %w", err)
			logger.Warnf(err.Error())
			return err
		}
	}
	if _, err := run(logger, "", "config", "http.sslVerify", strconv.FormatBool(spec.SSLVerify)); err != nil {
		logger.Warnf("Failed to set http.sslVerify in git config: %s", err)
		return err
	}

	fetchArgs := []string{"fetch"}
	if spec.Submodules {
		fetchArgs = append(fetchArgs, "--recurse-submodules=yes")
	}
	if spec.Depth > 0 {
		fetchArgs = append(fetchArgs, fmt.Sprintf("--depth=%d", spec.Depth))

		// Prevent fetching of unrelated git objects with shallow clones.
		if _, err := run(logger, "", "config", "--unset", "remote.origin.fetch"); err != nil {
			logger.Warnf("Failed to unset remote.origin.fetch in git config: %s", err)
		}
	}

	// Fetch the revision and verify with FETCH_HEAD
	fetchParam := []string{spec.Revision}
	checkoutParam := "FETCH_HEAD"

	if spec.Refspec != "" {
		// if refspec is specified, fetch the refspec and verify with provided revision
		fetchParam = strings.Split(spec.Refspec, " ")
		checkoutParam = spec.Revision
	}

	// git-init always creates and checks out an empty master branch. When the user requests
	// "master" as the revision, git-fetch will refuse to update the HEAD of the branch it is
	// currently on. The --update-head-ok parameter tells git-fetch that it is ok to update
	// the current (empty) HEAD on initial fetch.
	// The --force parameter tells git-fetch that its ok to update an existing HEAD in a
	// non-fast-forward manner (though this cannot be possible on initial fetch, it can help
	// when the refspec specifies the same destination twice)
	fetchArgs = append(fetchArgs, "origin", "--update-head-ok", "--force")
	fetchArgs = append(fetchArgs, fetchParam...)
	if _, _, err := retryWithBackoff(
		func() (string, error) { return run(logger, spec.Path, fetchArgs...) },
		retryConfig.Initial,
		retryConfig.Max,
		retryConfig.Factor,
		retryConfig.MaxAttempts,
		logger,
	); err != nil {
		return fmt.Errorf("failed to fetch %v: %w", fetchParam, err)
	}
	// After performing a fetch, verify that the item to checkout is actually valid
	if _, err := ShowCommit(logger, checkoutParam, spec.Path); err != nil {
		return fmt.Errorf("error parsing %s after fetching refspec %s: %w", checkoutParam, spec.Refspec, err)
	}

	if _, err := run(logger, "", "checkout", "-f", checkoutParam); err != nil {
		return err
	}

	commit, err := ShowCommit(logger, "HEAD", spec.Path)
	if err != nil {
		return err
	}
	ref, err := showRef(logger, "HEAD", spec.Path)
	if err != nil {
		return err
	}
	logger.Infof("Successfully cloned %s @ %s (%s) in path %s", trimmedURL, commit, ref, spec.Path)
	if spec.Submodules {
		if err := submoduleFetch(logger, spec, retryConfig); err != nil {
			return err
		}
	}
	return nil
}

// ShowCommit calls "git show ..." to get the commit SHA for the given revision
func ShowCommit(logger *zap.SugaredLogger, revision, path string) (string, error) {
	output, err := run(logger, path, "show", "-q", "--pretty=format:%H", revision)
	if err != nil {
		return "", err
	}
	return strings.TrimSuffix(output, "\n"), nil
}

func showRef(logger *zap.SugaredLogger, revision, path string) (string, error) {
	output, err := run(logger, path, "show", "-q", "--pretty=format:%D", revision)
	if err != nil {
		return "", err
	}
	return strings.TrimSuffix(output, "\n"), nil
}

func buildSubmoduleUpdateArgs(spec FetchSpec) []string {
	updateArgs := []string{"submodule", "update", "--recursive", "--init", "--force"}
	if spec.Depth > 0 {
		updateArgs = append(updateArgs, fmt.Sprintf("--depth=%d", spec.Depth))
	}
	if len(spec.SubmodulePaths) > 0 {
		updateArgs = append(updateArgs, spec.SubmodulePaths...)
	}
	return updateArgs
}

func submoduleFetch(logger *zap.SugaredLogger, spec FetchSpec, retryConfig RetryConfig) error {
	if spec.Path != "" {
		if err := os.Chdir(spec.Path); err != nil {
			return fmt.Errorf("failed to change directory with path %s; err: %w", spec.Path, err)
		}
	}
	updateArgs := buildSubmoduleUpdateArgs(spec)
	if _, _, err := retryWithBackoff(
		func() (string, error) { return run(logger, "", updateArgs...) },
		retryConfig.Initial,
		retryConfig.Max,
		retryConfig.Factor,
		retryConfig.MaxAttempts,
		logger,
	); err != nil {
		return err
	}
	logger.Infof("Successfully initialized and updated submodules in path %s", spec.Path)
	return nil
}

// ensureHomeEnv works around an issue where ssh doesn't respect the HOME env variable. If HOME is set and
// different from the user's detected home directory then symlink .ssh from the home directory to the HOME env
// var. This way ssh will see the .ssh directory in the user's home directory even though it ignores
// the HOME env var.
func ensureHomeEnv(logger *zap.SugaredLogger, homepath string) {
	homeenv := os.Getenv("HOME")
	if _, err := os.Stat(filepath.Join(homeenv, ".ssh")); err != nil {
		// There's no $HOME/.ssh directory to access or the user doesn't have permissions
		// to read it, or something else; in any event there's no need to try creating a
		// symlink to it.
		return
	}
	if homeenv != "" {
		ensureHomeEnvSSHLinkedFromPath(logger, homeenv, homepath)
	}
}

func ensureHomeEnvSSHLinkedFromPath(logger *zap.SugaredLogger, homeenv, homepath string) {
	if filepath.Clean(homeenv) != filepath.Clean(homepath) {
		homeEnvSSH := filepath.Join(homeenv, ".ssh")
		homePathSSH := filepath.Join(homepath, ".ssh")
		if _, err := os.Stat(homePathSSH); os.IsNotExist(err) {
			if err := os.Symlink(homeEnvSSH, homePathSSH); err != nil {
				// Only do a warning, in case we don't have a real home
				// directory writable in our image
				logger.Warnf("Unexpected error: creating symlink: %v", err)
			}
		}
	}
}

func userHasKnownHostsFile(homepath string) (bool, error) {
	f, err := os.Open(filepath.Join(homepath, sshKnownHostsUserPath))
	if err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, err
	}
	defer func() { _ = f.Close() }()
	return true, nil
}

func validateGitAuth(logger *zap.SugaredLogger, credsDir, url string) {
	sshCred := true
	if _, err := os.Stat(filepath.Join(credsDir, ".ssh")); os.IsNotExist(err) {
		sshCred = false
	}
	urlSSHFormat := validateGitSSHURLFormat(url)
	if sshCred && !urlSSHFormat {
		logger.Warnf("SSH credentials have been provided but the URL(%q) is not a valid SSH URL. This warning can be safely ignored if the URL is for a public repo or you are using basic auth", url)
	} else if !sshCred && urlSSHFormat {
		logger.Warnf("URL(%q) appears to need SSH authentication but no SSH credentials have been provided", url)
	}
}

// validateGitSSHURLFormat validates the given URL format is SSH or not
func validateGitSSHURLFormat(url string) bool {
	if strings.HasPrefix(url, "http://") || strings.HasPrefix(url, "https://") {
		return false
	}
	return sshURLRegexFormat.MatchString(url)
}

func configSparseCheckout(logger *zap.SugaredLogger, spec FetchSpec) error {
	if spec.SparseCheckoutDirectories != "" {
		if _, err := run(logger, "", "config", "core.sparsecheckout", "true"); err != nil {
			return err
		}

		dirPatterns := strings.Split(spec.SparseCheckoutDirectories, ",")

		cwd, err := os.Getwd()
		if err != nil {
			logger.Errorf("failed to get current directory: %v", err)
			return err
		}
		file, err := os.OpenFile(filepath.Join(cwd, ".git/info/sparse-checkout"), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
		if err != nil {
			logger.Errorf("failed to open sparse-checkout file: %v", err)
			return err
		}
		for _, pattern := range dirPatterns {
			if _, err := file.WriteString(pattern + "\n"); err != nil {
				defer func() { _ = file.Close() }()
				logger.Errorf("failed to write to sparse-checkout file: %v", err)
				return err
			}
		}
		if err := file.Close(); err != nil {
			logger.Errorf("failed to close sparse-checkout file: %v", err)
			return err
		}
	}
	return nil
}

type operation[T any] func() (T, error)

// retryWithBackoff runs `operation` until it succeeds or the context is done,
// with exponential backoff and jitter between retries.
func retryWithBackoff[T any](
	operation operation[T],
	initial time.Duration,
	max time.Duration,
	factor float64,
	maxAttempts int,
	logger *zap.SugaredLogger,
) (T, time.Duration, error) {

	waitTime := time.Duration(0)

	for attempt := 0; ; attempt++ {
		logger.Infof("Retrying operation (attempt %d)", attempt+1)
		result, err := operation()
		if err == nil {
			return result, waitTime, nil
		}

		if attempt+1 == maxAttempts {
			return result, waitTime, err
		}

		// compute backoff: exponential
		backoff := min(time.Duration(float64(initial)*math.Pow(factor, float64(attempt))), max)
		// add jitter: random in [0, next)
		jitter := time.Duration(rand.Int63n(int64(backoff)))
		wait := backoff + jitter/2
		time.Sleep(wait)
		waitTime += wait
	}
}

var credentialURLPattern = regexp.MustCompile(`(https?://)([^@]+)@`)

func redactCredentials(s string) string {
	return credentialURLPattern.ReplaceAllString(s, "${1}****@")
}

type errorHint struct {
	pattern string
	hint    string
}

var errorHints = []errorHint{
	{"could not read username", `The repository may be private. Configure a "basic-auth" or "ssh-directory" workspace to provide credentials.`},
	{"authentication failed", `The provided credentials were rejected. Verify your "basic-auth" or "ssh-directory" workspace contains valid credentials.`},
	{"permission denied (publickey)", `The SSH key was rejected. Ensure the "ssh-directory" workspace contains a valid private key with access to the repository.`},
	{"repository not found", `The repository does not exist or you don't have access. Check the URL and ensure credentials are configured via "basic-auth" or "ssh-directory" workspace if the repository is private.`},
	{"could not resolve host", `Cannot reach the git server. Check the URL for typos, verify network connectivity, and check "httpProxy"/"httpsProxy" settings if behind a proxy.`},
	{"ssl certificate problem", `TLS/SSL verification failed. If using a self-signed certificate, provide it via the "ssl-ca-directory" workspace, or set "sslVerify" to "false" (not recommended).`},
	{"server certificate verification failed", `TLS/SSL verification failed. If using a self-signed certificate, provide it via the "ssl-ca-directory" workspace, or set "sslVerify" to "false" (not recommended).`},
	{"couldn't find remote ref", "The revision, branch, or tag was not found on the remote. Verify the name exists on the repository."},
	{"upload-pack: not our ref", "The requested commit SHA does not exist on the remote. It may have been force-pushed over or garbage-collected."},
	{"reference is not a tree", "The requested commit SHA does not exist on the remote. It may have been force-pushed over or garbage-collected."},
	{"bad object", "The requested revision does not exist. It may have been force-pushed over or garbage-collected."},
	{"connection refused", "The git server refused the connection. Verify the URL and that the server is reachable from the cluster."},
	{"connection timed out", `Connection timed out. Check network/firewall rules and "httpProxy"/"httpsProxy"/"noProxy" settings.`},
	{"failed to connect", `Cannot connect to the git server. Check network/firewall rules and "httpProxy"/"httpsProxy"/"noProxy" settings.`},
	{"the remote end hung up unexpectedly", `Transfer was interrupted. Try increasing "retryMaxAttempts" or check network stability. For large repositories, consider using a shallow clone with "depth".`},
	{"rpc failed", `Transfer was interrupted. Try increasing "retryMaxAttempts" or check network stability. For large repositories, consider using a shallow clone with "depth".`},
	{"sparse checkout leaves no entry", `The sparse checkout pattern matched no files. Verify the "sparseCheckoutDirectories" parameter.`},
	{"transport 'file' not allowed", "The git file:// protocol is blocked. This can happen with submodules pointing to local paths."},
}

// FormatUserFriendlyError produces a human-readable error message with
// contextual hints and reproduction commands.
func FormatUserFriendlyError(spec FetchSpec, err error) string {
	var gitErr *GitError
	var sb strings.Builder

	sb.WriteString("\n========================================\n")
	sb.WriteString("Git Clone Failed\n")
	sb.WriteString("========================================\n\n")

	errOutput := ""
	if errors.As(err, &gitErr) {
		errOutput = strings.TrimSpace(gitErr.Output)
	}
	if errOutput != "" {
		sb.WriteString("Error:\n  " + redactCredentials(errOutput) + "\n\n")
	} else {
		sb.WriteString("Error:\n  " + redactCredentials(err.Error()) + "\n\n")
	}

	fullText := strings.ToLower(errOutput + " " + err.Error())
	for _, h := range errorHints {
		if strings.Contains(fullText, h.pattern) {
			sb.WriteString("Hint:\n  " + h.hint + "\n\n")
			break
		}
	}

	url := redactCredentials(spec.URL)
	sb.WriteString("To reproduce locally, run:\n\n")
	sb.WriteString("  git init <dir> && cd <dir>\n")
	fmt.Fprintf(&sb, "  git remote add origin %s\n", url)

	fetchCmd := "  git fetch origin"
	if spec.Depth > 0 {
		fetchCmd += fmt.Sprintf(" --depth=%d", spec.Depth)
	}
	if spec.Refspec != "" {
		fetchCmd += " " + spec.Refspec
	} else if spec.Revision != "" {
		fetchCmd += " " + spec.Revision
	}
	sb.WriteString(fetchCmd + "\n")

	if spec.Revision != "" {
		fmt.Fprintf(&sb, "  git checkout %s\n", spec.Revision)
	} else {
		sb.WriteString("  git checkout FETCH_HEAD\n")
	}

	sb.WriteString("\n========================================\n")
	return sb.String()
}
