/*
Copyright 2020 The Tekton Authors

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
	"bufio"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"
)

const fileMode = 0755 // rwxr-xr-x

func withTemporaryGitConfig(t *testing.T) {
	t.Helper()
	gitConfigDir := t.TempDir()
	key := "GIT_CONFIG_GLOBAL"
	t.Setenv(key, filepath.Join(gitConfigDir, "config"))
}

func TestValidateGitSSHURLFormat(t *testing.T) {
	withTemporaryGitConfig(t)
	tests := []struct {
		url  string
		want bool
	}{
		{
			url:  "git@github.com:user/project.git",
			want: true,
		},
		{
			url:  "git@127.0.0.1:user/project.git",
			want: true,
		},
		{
			url:  "http://github.com/user/project.git",
			want: false,
		},
		{
			url:  "https://github.com/user/project.git",
			want: false,
		},
		{
			url:  "http://127.0.0.1/user/project.git",
			want: false,
		},
		{
			url:  "https://127.0.0.1/user/project.git",
			want: false,
		},
		{
			url:  "http://host.xz/path/to/repo.git/",
			want: false,
		},
		{
			url:  "https://host.xz/path/to/repo.git/",
			want: false,
		},
		{
			url:  "https://host.xz:1443/path/to/repo.git/",
			want: false,
		},
		{
			url:  "ssh://user@host.xz:port/path/to/repo.git/",
			want: true,
		},
		{
			url:  "ssh://user@host.xz/path/to/repo.git/",
			want: true,
		},
		{
			url:  "ssh://host.xz:port/path/to/repo.git/",
			want: true,
		},
		{
			url:  "ssh://host.xz/path/to/repo.git/",
			want: true,
		},
		{
			url:  "git://host.xz/path/to/repo.git/",
			want: false,
		},
		{
			url:  "/path/to/repo.git/",
			want: false,
		},
		{
			url:  "file://~/path/to/repo.git/",
			want: false,
		},
		{
			url:  "user@host.xz:/path/to/repo.git/",
			want: true,
		},
		{
			url:  "host.xz:/path/to/repo.git/",
			want: true,
		},
		{
			url:  "user@host.xz:path/to/repo.git",
			want: true,
		},
	}

	for _, tt := range tests {
		got := validateGitSSHURLFormat(tt.url)
		if got != tt.want {
			t.Errorf("Validate URL(%v)'s SSH format got %v, want %v", tt.url, got, tt.want)
		}
	}
}

func TestValidateGitAuth(t *testing.T) {
	withTemporaryGitConfig(t)
	tests := []struct {
		name       string
		url        string
		logMessage string
		wantSSHdir bool
	}{
		{
			name:       "Valid HTTP Auth",
			url:        "http://google.com",
			logMessage: "",
			wantSSHdir: false,
		},
		{
			name:       "Valid SSH Auth",
			url:        "ssh://git@github.com:chmouel/tekton",
			logMessage: "",
			wantSSHdir: true,
		},
		{
			name:       "SSH URL but no SSH credentials",
			url:        "ssh://git@github.com:chmouel/tekton",
			logMessage: "URL(\"ssh://git@github.com:chmouel/tekton\") appears to need SSH authentication but no SSH credentials have been provided",
			wantSSHdir: false,
		},
		{
			name:       "Invalid SSH URL",
			url:        "http://github.com/chmouel/tekton",
			logMessage: "SSH credentials have been provided but the URL(\"http://github.com/chmouel/tekton\") is not a valid SSH URL. This warning can be safely ignored if the URL is for a public repo or you are using basic auth",
			wantSSHdir: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			observer, log := observer.New(zap.InfoLevel)
			logger := zap.New(observer).Sugar()
			credsDir := t.TempDir()
			if tt.wantSSHdir {
				err := os.MkdirAll(filepath.Join(credsDir, ".ssh"), fileMode)
				if err != nil {
					t.Errorf("Error creating SSH dir: %v", err)
				}
			}

			validateGitAuth(logger, credsDir, tt.url)
			checkLogMessage(t, tt.logMessage, log, 0)
		})
	}
}

func TestUserHasKnownHostsFile(t *testing.T) {
	withTemporaryGitConfig(t)
	tests := []struct {
		name               string
		want               bool
		wantKnownHostsFile bool
	}{
		{
			name:               "known-hosts-file-exists",
			want:               true,
			wantKnownHostsFile: true,
		},
		{
			name:               "known-hosts-file-doesnt-exist",
			want:               false,
			wantKnownHostsFile: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			homedir := t.TempDir()
			if tt.wantKnownHostsFile {
				os.MkdirAll(filepath.Join(homedir, ".ssh"), fileMode)
				knownHostsFile := filepath.Join(homedir, sshKnownHostsUserPath)
				_, err := os.Create(knownHostsFile)
				if err != nil {
					t.Fatalf("Could not create test file %s: %v", knownHostsFile, err)
				}
			}
			got, _ := userHasKnownHostsFile(homedir)
			if got != tt.want {
				t.Errorf("User has known hosts file got %v, want %v", got, tt.want)
			}
		})
	}
}

func TestEnsureHomeEnv(t *testing.T) {
	withTemporaryGitConfig(t)
	tests := []struct {
		name                 string
		homeenvSet           bool
		homeenvEqualsHomedir bool
	}{
		{
			name:                 "Homeenv not set",
			homeenvSet:           false,
			homeenvEqualsHomedir: true,
		},
		{
			name:                 "Homeenv same as homedir",
			homeenvSet:           true,
			homeenvEqualsHomedir: true,
		},
		{
			name:                 "Homeenv different from homedir",
			homeenvSet:           true,
			homeenvEqualsHomedir: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			observer, _ := observer.New(zap.InfoLevel)
			logger := zap.New(observer).Sugar()
			homedir := t.TempDir()
			var homeenv string
			if tt.homeenvEqualsHomedir {
				homeenv = homedir
			} else {
				homeenv = t.TempDir()
			}
			if tt.homeenvSet {
				t.Setenv("HOME", homeenv)
			}
			// Create SSH creds directory in directory specified by HOME envvar
			if err := os.MkdirAll(filepath.Join(homeenv, ".ssh"), fileMode); err != nil {
				t.Fatalf("Error creating SSH creds in homeenv dir %s: %v", homeenv, err)
			}

			ensureHomeEnv(logger, homedir)

			// Ensure SSH creds file present in detected home directory
			if _, err := os.Stat(filepath.Join(homedir, ".ssh")); os.IsNotExist(err) {
				t.Errorf("SSH creds not present in homedir %s", homedir)
			}
		})
	}
}

func TestFetch(t *testing.T) {
	withTemporaryGitConfig(t)
	tests := []struct {
		name       string
		logMessage string
		spec       FetchSpec
		wantErr    bool
	}{
		{
			name:       "test-good",
			logMessage: "Successfully cloned",
			wantErr:    false,
			spec: FetchSpec{
				URL:                       "",
				Revision:                  "",
				Refspec:                   "",
				Path:                      "",
				Depth:                     0,
				Submodules:                false,
				SSLVerify:                 false,
				HTTPProxy:                 "",
				HTTPSProxy:                "",
				NOProxy:                   "",
				SparseCheckoutDirectories: "",
			},
		}, {
			name:       "test-clone-with-sparse-checkout",
			logMessage: "Successfully cloned",
			wantErr:    false,
			spec: FetchSpec{
				URL:                       "",
				Revision:                  "",
				Refspec:                   "",
				Path:                      "",
				Depth:                     0,
				Submodules:                false,
				SSLVerify:                 false,
				HTTPProxy:                 "",
				HTTPSProxy:                "",
				NOProxy:                   "",
				SparseCheckoutDirectories: "a,b/c",
			},
		}, {
			name:       "test-clone-with-submodules",
			logMessage: "updated submodules",
			wantErr:    false,
			spec: FetchSpec{
				URL:                       "",
				Revision:                  "",
				Refspec:                   "",
				Path:                      "",
				Depth:                     0,
				Submodules:                true,
				HTTPProxy:                 "",
				HTTPSProxy:                "",
				NOProxy:                   "",
				SparseCheckoutDirectories: "",
			},
		}, {
			name:       "test-clone-with-submodules-empty-paths",
			logMessage: "updated submodules",
			wantErr:    false,
			spec: FetchSpec{
				URL:            "",
				Revision:       "",
				Refspec:        "",
				Path:           "",
				Depth:          0,
				Submodules:     true,
				SubmodulePaths: []string{},
				HTTPProxy:      "",
				HTTPSProxy:     "",
				NOProxy:        "",
			},
		}, {
			name:       "test-clone-with-submodules-defined-paths",
			logMessage: "updated submodules",
			wantErr:    false,
			spec: FetchSpec{
				URL:            "",
				Revision:       "",
				Refspec:        "",
				Path:           "",
				Depth:          0,
				Submodules:     true,
				SubmodulePaths: []string{"test_submod"},
				HTTPProxy:      "",
				HTTPSProxy:     "",
				NOProxy:        "",
			},
		}, {
			name:       "test-clone-with-depth",
			logMessage: "Successfully cloned",
			wantErr:    false,
			spec: FetchSpec{
				URL:                       "",
				Revision:                  "",
				Refspec:                   "",
				Path:                      "",
				Depth:                     1,
				Submodules:                false,
				SSLVerify:                 false,
				HTTPProxy:                 "",
				HTTPSProxy:                "",
				NOProxy:                   "",
				SparseCheckoutDirectories: "",
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			observer, log := observer.New(zap.InfoLevel)
			defer func() {
				for _, line := range log.TakeAll() {
					t.Logf("[%q git]: %s", line.Level, line.Message)
				}
			}()
			logger := zap.New(observer).Sugar()
			logLine := 1

			submodPath := ""
			submodName := "default"
			if tt.spec.Submodules {
				submodPath = t.TempDir()
				createTempGit(t, logger, submodPath, "", "")
			}

			if len(tt.spec.SubmodulePaths) > 0 {
				// test submodule path, replace template value
				submodName = tt.spec.SubmodulePaths[0]
			}

			gitDir := t.TempDir()
			createTempGit(t, logger, gitDir, submodPath, submodName)
			tt.spec.URL = gitDir

			targetPath := t.TempDir()
			tt.spec.Path = targetPath

			if err := Fetch(logger, tt.spec, RetryConfig{
				Initial:     1 * time.Second,
				Max:         10 * time.Second,
				Factor:      2.0,
				MaxAttempts: 3,
			}); (err != nil) != tt.wantErr {
				t.Errorf("Fetch() error = %v, wantErr %v", err, tt.wantErr)
			}

			if tt.spec.SparseCheckoutDirectories != "" {
				dirPatterns := strings.Split(tt.spec.SparseCheckoutDirectories, ",")

				sparseFile, err := os.Open(".git/info/sparse-checkout")
				if err != nil {
					t.Fatal("Unable to read sparse-checkout file")
				}
				defer sparseFile.Close()

				var sparsePatterns []string

				scanner := bufio.NewScanner(sparseFile)
				for scanner.Scan() {
					sparsePatterns = append(sparsePatterns, scanner.Text())
				}

				if cmp.Diff(dirPatterns, sparsePatterns) != "" {
					t.Errorf("directory patterns and sparse-checkout patterns do not match")
				}
			}

			if tt.spec.Depth > 0 {
				shallowFile, err := os.Open(".git/shallow")
				if err != nil {
					t.Fatal("Faile to read shallow file")
				}
				defer shallowFile.Close()

				var commitCount int
				scanner := bufio.NewScanner(shallowFile)
				for scanner.Scan() {
					commitCount++
				}
				if commitCount != int(tt.spec.Depth) {
					t.Errorf("Expected %d commits in shallow file, got %d", tt.spec.Depth, commitCount)
				}

				// Verify remote.origin.fetch was unset
				_, err = run(logger, "", "config", "--get", "remote.origin.fetch")
				if err == nil {
					t.Error("git fetch config should be unset for a shallow clone")
				}
			}

			if tt.spec.Submodules {
				submoduleDirs, err := filepath.Glob(".git/modules/*")
				if err != nil {
					t.Fatalf("Error finding submodule directories: %v", err)
				}

				if len(submoduleDirs) == 0 {
					t.Error("No cloned submodules found")
				}
				logLine = 3
			}

			checkLogMessage(t, tt.logMessage, log, logLine)
		})
	}
}

// Create a temporary Git dir locally for testing against instead of using a potentially flaky remote URL.
func createTempGit(t *testing.T, logger *zap.SugaredLogger, gitDir string, submodPath string, submodName string) {
	t.Helper()
	if _, err := run(logger, "", "init", gitDir); err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(gitDir); err != nil {
		t.Fatalf("failed to change directory with path %s; err: %v", gitDir, err)
	}
	if _, err := run(logger, "", "checkout", "-b", "main"); err != nil {
		t.Fatal(err)
	}

	// Not defining globally so we don't mess with the global gitconfig
	if _, err := run(logger, "", "config", "user.email", "tester@tekton.dev"); err != nil {
		t.Fatal(err)
	}

	// Not defining globally so we don't mess with the global gitconfig
	if _, err := run(logger, "", "config", "user.name", "Tekton Test"); err != nil {
		t.Fatal(err)
	}

	if _, err := run(logger, "", "commit", "--allow-empty", "-m", "Hello Moto"); err != nil {
		t.Fatal(err.Error())
	}

	if submodPath != "" {
		// file protocol is necessary to clone submodules since the fixture is only written to the filesystem
		if _, err := run(logger, "", "config", "--global", "protocol.file.allow", "always"); err != nil {
			t.Fatal(err)
		}

		if submodName != "" {
			if _, err := run(logger, "", "submodule", "add", submodPath, submodName); err != nil {
				t.Fatal(err.Error())
			}
		} else {
			if _, err := run(logger, "", "submodule", "add", submodPath); err != nil {
				t.Fatal(err.Error())
			}
		}

		if _, err := run(logger, "", "add", "."); err != nil {
			t.Fatal(err.Error())
		}
		if _, err := run(logger, "", "commit", "-m", "Add submodule"); err != nil {
			t.Fatal(err.Error())
		}
	}
}

func checkLogMessage(t *testing.T, logMessage string, log *observer.ObservedLogs, logLine int) {
	t.Helper()
	if logMessage == "" {
		return
	}
	allLogLines := log.All()
	if len(allLogLines) == 0 {
		t.Fatal("We didn't receive any logging")
	}
	gotmsg := allLogLines[logLine].Message
	if !strings.Contains(gotmsg, logMessage) {
		t.Errorf("log message: '%s'\n should contain: '%s'", logMessage, gotmsg)
	}
}

type SucceedAfter struct {
	try       int
	callCount int
}

func (f *SucceedAfter) Run() (string, error) {
	f.callCount++
	if f.callCount > f.try {
		return "success", nil
	}
	return "", fmt.Errorf("temporary error")
}

func TestBuildSubmoduleUpdateArgs(t *testing.T) {
	tests := []struct {
		name     string
		spec     FetchSpec
		expected []string
	}{
		{
			name: "no depth, no submodule paths",
			spec: FetchSpec{
				Depth:          0,
				SubmodulePaths: nil,
			},
			expected: []string{"submodule", "update", "--recursive", "--init", "--force"},
		},
		{
			name: "with depth, no submodule paths",
			spec: FetchSpec{
				Depth:          5,
				SubmodulePaths: nil,
			},
			expected: []string{"submodule", "update", "--recursive", "--init", "--force", "--depth=5"},
		},
		{
			name: "no depth, with submodule paths",
			spec: FetchSpec{
				Depth:          0,
				SubmodulePaths: []string{"path/to/submod1", "path/to/submod2"},
			},
			expected: []string{"submodule", "update", "--recursive", "--init", "--force", "path/to/submod1", "path/to/submod2"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := buildSubmoduleUpdateArgs(tt.spec)
			if diff := cmp.Diff(tt.expected, got); diff != "" {
				t.Errorf("buildSubmoduleUpdateArgs() mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

func TestRetryWithBackoff(t *testing.T) {
	withTemporaryGitConfig(t)
	tests := []struct {
		name           string
		operation      func() (string, error)
		initial        time.Duration
		max            time.Duration
		factor         float64
		maxAttempts    int
		expectedResult string
		expectedError  bool
		expectedMin    time.Duration
		expectedMax    time.Duration
	}{
		{
			name:           "successful operation on first attempt",
			operation:      (&SucceedAfter{try: 0}).Run,
			initial:        100 * time.Millisecond,
			max:            1 * time.Second,
			factor:         2.0,
			maxAttempts:    3,
			expectedResult: "success",
			expectedError:  false,
			expectedMin:    0,
			expectedMax:    50 * time.Millisecond,
		},
		{
			name:           "successful operation on second attempt",
			operation:      (&SucceedAfter{try: 1}).Run,
			initial:        100 * time.Millisecond,
			max:            1 * time.Second,
			factor:         2.0,
			maxAttempts:    3,
			expectedResult: "success",
			expectedError:  false,
			expectedMin:    100 * time.Millisecond,
			expectedMax:    200 * time.Millisecond,
		},
		{
			name:           "operation fails after max attempts",
			operation:      (&SucceedAfter{try: 10}).Run,
			initial:        100 * time.Millisecond,
			max:            1 * time.Second,
			factor:         2.0,
			maxAttempts:    2,
			expectedResult: "",
			expectedError:  true,
			expectedMin:    100 * time.Millisecond,
			expectedMax:    200 * time.Millisecond,
		},
		{
			name:           "operation fails and max backoff is reached",
			operation:      (&SucceedAfter{try: 10}).Run,
			initial:        100 * time.Millisecond,
			max:            1 * time.Millisecond,
			factor:         2.0,
			maxAttempts:    3,
			expectedResult: "",
			expectedError:  true,
			expectedMin:    2 * time.Millisecond,
			expectedMax:    4 * time.Millisecond,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			observer, log := observer.New(zap.InfoLevel)
			logger := zap.New(observer).Sugar()

			result, waitTime, err := retryWithBackoff(
				tt.operation,
				tt.initial,
				tt.max,
				tt.factor,
				tt.maxAttempts,
				logger,
			)

			if tt.expectedError {
				if err == nil {
					t.Errorf("Expected error but got none")
				}
			} else {
				if err != nil {
					t.Errorf("Unexpected error: %v", err)
				}
				if result != tt.expectedResult {
					t.Errorf("Expected result %q, got %q", tt.expectedResult, result)
				}
			}

			// Verify that retry attempts were logged
			logLines := log.All()
			if len(logLines) == 0 {
				t.Error("Expected retry logs but got none")
			}

			// Assert expected duration
			if waitTime < tt.expectedMin {
				t.Errorf("Expected duration >= %v, got %v", tt.expectedMin, waitTime)
			}
			if waitTime > tt.expectedMax {
				t.Errorf("Expected duration <= %v, got %v", tt.expectedMax, waitTime)
			}
		})
	}
}

func TestGitError_Error(t *testing.T) {
	tests := []struct {
		name     string
		gitErr   GitError
		expected string
	}{
		{
			name: "with output",
			gitErr: GitError{
				Args:   []string{"fetch", "origin"},
				Output: "fatal: could not read Username\n",
				Err:    fmt.Errorf("exit status 128"),
			},
			expected: "exit status 128: fatal: could not read Username",
		},
		{
			name: "without output",
			gitErr: GitError{
				Args: []string{"fetch", "origin"},
				Err:  fmt.Errorf("exit status 128"),
			},
			expected: "exit status 128",
		},
		{
			name: "empty output",
			gitErr: GitError{
				Args:   []string{"fetch"},
				Output: "  \n  ",
				Err:    fmt.Errorf("exit status 1"),
			},
			expected: "exit status 1",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.gitErr.Error()
			if got != tt.expected {
				t.Errorf("GitError.Error() = %q, want %q", got, tt.expected)
			}
		})
	}
}

func TestGitError_Unwrap(t *testing.T) {
	inner := fmt.Errorf("exit status 128")
	gitErr := &GitError{Err: inner}
	wrapped := fmt.Errorf("failed to fetch: %w", gitErr)

	var target *GitError
	if !errors.As(wrapped, &target) {
		t.Fatal("errors.As should find GitError in wrapped error chain")
	}
	if target.Err != inner {
		t.Errorf("unwrapped inner error = %v, want %v", target.Err, inner)
	}
}

func TestRedactCredentials(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "https with credentials",
			input:    "https://user:pass@github.com/org/repo",
			expected: "https://****@github.com/org/repo",
		},
		{
			name:     "http with credentials",
			input:    "http://token@gitlab.com/org/repo",
			expected: "http://****@gitlab.com/org/repo",
		},
		{
			name:     "no credentials",
			input:    "https://github.com/org/repo",
			expected: "https://github.com/org/repo",
		},
		{
			name:     "ssh url unchanged",
			input:    "git@github.com:org/repo.git",
			expected: "git@github.com:org/repo.git",
		},
		{
			name:     "credentials in error message",
			input:    "fatal: Authentication failed for 'https://user:secret@github.com/org/repo'",
			expected: "fatal: Authentication failed for 'https://****@github.com/org/repo'",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := redactCredentials(tt.input)
			if got != tt.expected {
				t.Errorf("redactCredentials(%q) = %q, want %q", tt.input, got, tt.expected)
			}
		})
	}
}

func TestFormatUserFriendlyError(t *testing.T) {
	tests := []struct {
		name            string
		spec            FetchSpec
		err             error
		wantContains    []string
		wantNotContains []string
	}{
		{
			name: "auth error with hint",
			spec: FetchSpec{
				URL:      "https://github.com/org/repo",
				Revision: "abc123",
				Depth:    1,
			},
			err: &GitError{
				Output: "fatal: could not read Username for 'https://github.com': No such device or address\n",
				Err:    fmt.Errorf("exit status 128"),
			},
			wantContains: []string{
				"Git Clone Failed",
				"could not read Username",
				"basic-auth",
				"ssh-directory",
				"git init <dir>",
				"git remote add origin https://github.com/org/repo",
				"git fetch origin --depth=1 abc123",
				"git checkout abc123",
			},
		},
		{
			name: "unknown error without hint",
			spec: FetchSpec{
				URL:   "https://github.com/org/repo",
				Depth: 1,
			},
			err: fmt.Errorf("some unknown error"),
			wantContains: []string{
				"Git Clone Failed",
				"some unknown error",
				"git fetch origin --depth=1",
				"git checkout FETCH_HEAD",
			},
			wantNotContains: []string{
				"Hint:",
			},
		},
		{
			name: "case insensitive hint matching",
			spec: FetchSpec{
				URL:   "https://github.com/org/repo",
				Depth: 0,
			},
			err: &GitError{
				Output: "FATAL: COULD NOT READ USERNAME\n",
				Err:    fmt.Errorf("exit status 128"),
			},
			wantContains: []string{
				"Hint:",
				"basic-auth",
			},
		},
		{
			name: "credentials redacted in output",
			spec: FetchSpec{
				URL:   "https://user:secret@github.com/org/repo",
				Depth: 1,
			},
			err: &GitError{
				Output: "fatal: Authentication failed for 'https://user:secret@github.com/org/repo'\n",
				Err:    fmt.Errorf("exit status 128"),
			},
			wantContains: []string{
				"****@github.com",
			},
			wantNotContains: []string{
				"user:secret",
			},
		},
		{
			name: "ssl certificate error",
			spec: FetchSpec{
				URL:   "https://git.internal.com/repo",
				Depth: 1,
			},
			err: &GitError{
				Output: "fatal: unable to access: SSL certificate problem: self-signed certificate\n",
				Err:    fmt.Errorf("exit status 128"),
			},
			wantContains: []string{
				"ssl-ca-directory",
				"sslVerify",
			},
		},
		{
			name: "connection refused",
			spec: FetchSpec{
				URL:   "https://git.example.com/repo",
				Depth: 1,
			},
			err: &GitError{
				Output: "fatal: unable to access: Failed to connect to git.example.com port 443: Connection refused\n",
				Err:    fmt.Errorf("exit status 128"),
			},
			wantContains: []string{
				"refused the connection",
			},
		},
		{
			name: "remote ref not found",
			spec: FetchSpec{
				URL:      "https://github.com/org/repo",
				Revision: "nonexistent-branch",
				Depth:    1,
			},
			err: &GitError{
				Output: "fatal: couldn't find remote ref nonexistent-branch\n",
				Err:    fmt.Errorf("exit status 128"),
			},
			wantContains: []string{
				"revision, branch, or tag was not found",
			},
		},
		{
			name: "refspec included in fetch command",
			spec: FetchSpec{
				URL:      "https://github.com/org/repo",
				Revision: "main",
				Refspec:  "refs/heads/main:refs/heads/main",
				Depth:    1,
			},
			err: &GitError{
				Output: "fatal: couldn't find remote ref\n",
				Err:    fmt.Errorf("exit status 128"),
			},
			wantContains: []string{
				"git fetch origin --depth=1 refs/heads/main:refs/heads/main",
				"git checkout main",
			},
			wantNotContains: []string{
				"git fetch origin --depth=1 main",
			},
		},
		{
			name: "depth zero omits depth flag",
			spec: FetchSpec{
				URL:      "https://github.com/org/repo",
				Revision: "main",
				Depth:    0,
			},
			err: fmt.Errorf("some error"),
			wantNotContains: []string{
				"--depth=",
			},
			wantContains: []string{
				"git fetch origin main",
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := FormatUserFriendlyError(tt.spec, tt.err)
			for _, want := range tt.wantContains {
				if !strings.Contains(got, want) {
					t.Errorf("FormatUserFriendlyError() missing %q in:\n%s", want, got)
				}
			}
			for _, notWant := range tt.wantNotContains {
				if strings.Contains(got, notWant) {
					t.Errorf("FormatUserFriendlyError() should not contain %q in:\n%s", notWant, got)
				}
			}
		})
	}
}
