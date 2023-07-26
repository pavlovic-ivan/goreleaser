package nix

import (
	"html/template"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/goreleaser/goreleaser/internal/artifact"
	"github.com/goreleaser/goreleaser/internal/client"
	"github.com/goreleaser/goreleaser/internal/golden"
	"github.com/goreleaser/goreleaser/internal/testctx"
	"github.com/goreleaser/goreleaser/pkg/config"
	"github.com/stretchr/testify/require"
)

func TestContinueOnError(t *testing.T) {
	require.True(t, Pipe{}.ContinueOnError())
}

func TestString(t *testing.T) {
	require.NotEmpty(t, Pipe{}.String())
}

func TestSkip(t *testing.T) {
	t.Run("no-nix", func(t *testing.T) {
		require.True(t, Pipe{}.Skip(testctx.New()))
	})
	t.Run("nix-all-good", func(t *testing.T) {
		require.False(t, NewPublish().Skip(testctx.NewWithCfg(config.Project{
			Nix: []config.Nix{{}},
		})))
	})
	t.Run("prefetcher-not-in-path", func(t *testing.T) {
		t.Setenv("PATH", "nope")
		require.True(t, NewPublish().Skip(testctx.NewWithCfg(config.Project{
			Nix: []config.Nix{{}},
		})))
	})
}

const fakeNixPrefetchURLBin = "fake-nix-prefetch-url"

func TestPrefetcher(t *testing.T) {
	t.Run("prefetch", func(t *testing.T) {
		t.Run("build", func(t *testing.T) {
			sha, err := buildShaPrefetcher{}.Prefetch("any")
			require.NoError(t, err)
			require.Equal(t, zeroHash, sha)
		})
		t.Run("publish", func(t *testing.T) {
			t.Run("no-nix-prefetch-url", func(t *testing.T) {
				_, err := publishShaPrefetcher{fakeNixPrefetchURLBin}.Prefetch("any")
				require.ErrorIs(t, err, exec.ErrNotFound)
			})
			t.Run("valid", func(t *testing.T) {
				sha, err := publishShaPrefetcher{nixPrefetchURLBin}.Prefetch("https://github.com/goreleaser/goreleaser/releases/download/v1.18.2/goreleaser_Darwin_arm64.tar.gz")
				require.NoError(t, err)
				require.Equal(t, "0girjxp07srylyq36xk1ska8p68m2fhp05xgyv4wkcl61d6rzv3y", sha)
			})
		})
	})
	t.Run("available", func(t *testing.T) {
		t.Run("build", func(t *testing.T) {
			require.True(t, buildShaPrefetcher{}.Available())
		})
		t.Run("publish", func(t *testing.T) {
			t.Run("no-nix-prefetch-url", func(t *testing.T) {
				require.False(t, publishShaPrefetcher{fakeNixPrefetchURLBin}.Available())
			})
			t.Run("valid", func(t *testing.T) {
				require.True(t, publishShaPrefetcher{nixPrefetchURLBin}.Available())
			})
		})
	})
}

func TestRunPipe(t *testing.T) {
	for _, tt := range []struct {
		name                 string
		expectRunErrorIs     error
		expectPublishErrorIs error
		nix                  config.Nix
	}{
		{
			name: "minimal",
			nix: config.Nix{
				Repository: config.RepoRef{
					Owner: "foo",
					Name:  "bar",
				},
			},
		},
		{
			name: "deps",
			nix: config.Nix{
				Repository: config.RepoRef{
					Owner: "foo",
					Name:  "bar",
				},
				Dependencies: []config.NixDependency{
					{Name: "fish"},
					{Name: "bash"},
					linuxDep("ttyd"),
					darwinDep("chromium"),
				},
			},
		},
		{
			name: "extra-install",
			nix: config.Nix{
				Repository: config.RepoRef{
					Owner: "foo",
					Name:  "bar",
				},
				Dependencies: []config.NixDependency{
					{Name: "fish"},
					{Name: "bash"},
					linuxDep("ttyd"),
					darwinDep("chromium"),
				},
				ExtraInstall: "installManPage ./manpages/foo.1.gz",
			},
		},
		{
			name: "open-pr",
			nix: config.Nix{
				Name:        "foo",
				IDs:         []string{"foo"},
				Description: "my test",
				Homepage:    "https://goreleaser.com",
				License:     "mit",
				Path:        "pkgs/foo.nix",
				Repository: config.RepoRef{
					Owner:  "foo",
					Name:   "bar",
					Branch: "update-{{.Version}}",
					PullRequest: config.PullRequest{
						Enabled: true,
					},
				},
			},
		},
		{
			name: "wrapped-in-dir",
			nix: config.Nix{
				Name:        "wrapped-in-dir",
				IDs:         []string{"wrapped-in-dir"},
				Description: "my test",
				Homepage:    "https://goreleaser.com",
				License:     "mit",
				PostInstall: `
					echo "do something"
				`,
				Install: `
					mkdir -p $out/bin
					cp foo $out/bin/foo
				`,
				Repository: config.RepoRef{
					Owner: "foo",
					Name:  "bar",
				},
			},
		},
		{
			name: "unibin",
			nix: config.Nix{
				Name:        "unibin",
				IDs:         []string{"unibin"},
				Description: "my test",
				Homepage:    "https://goreleaser.com",
				License:     "mit",
				Repository: config.RepoRef{
					Owner: "foo",
					Name:  "bar",
				},
			},
		},
		{
			name: "no-archives",
			expectRunErrorIs: errNoArchivesFound{
				goamd64: "v2",
				ids:     []string{"nopenopenope"},
			},
			nix: config.Nix{
				Name:    "no-archives",
				IDs:     []string{"nopenopenope"},
				Goamd64: "v2",
				Repository: config.RepoRef{
					Owner: "foo",
					Name:  "bar",
				},
			},
		},
		{
			name: "unibin-replaces",
			nix: config.Nix{
				Name:        "unibin-replaces",
				IDs:         []string{"unibin-replaces"},
				Description: "my test",
				Homepage:    "https://goreleaser.com",
				License:     "mit",
				Repository: config.RepoRef{
					Owner: "foo",
					Name:  "bar",
				},
			},
		},
		{
			name: "partial",
			nix: config.Nix{
				Name: "partial",
				IDs:  []string{"partial"},
				Repository: config.RepoRef{
					Owner: "foo",
					Name:  "bar",
				},
			},
		},
		{
			name:             "no-repo-name",
			expectRunErrorIs: errNoRepoName,
			nix: config.Nix{
				Name: "doesnotmatter",
				Repository: config.RepoRef{
					Owner: "foo",
				},
			},
		},
		{
			name:             "bad-name-tmpl",
			expectRunErrorIs: &template.Error{},
			nix: config.Nix{
				Name: "{{ .Nope }}",
				Repository: config.RepoRef{
					Owner: "foo",
					Name:  "bar",
				},
			},
		},
		{
			name:             "bad-description-tmpl",
			expectRunErrorIs: &template.Error{},
			nix: config.Nix{
				Description: "{{ .Nope }}",
				Repository: config.RepoRef{
					Owner: "foo",
					Name:  "bar",
				},
			},
		},
		{
			name:             "bad-homepage-tmpl",
			expectRunErrorIs: &template.Error{},
			nix: config.Nix{
				Homepage: "{{ .Nope }}",
				Repository: config.RepoRef{
					Owner: "foo",
					Name:  "bar",
				},
			},
		},
		{
			name:             "bad-repo-tmpl",
			expectRunErrorIs: &template.Error{},
			nix: config.Nix{
				Name: "doesnotmatter",
				Repository: config.RepoRef{
					Owner: "foo",
					Name:  "{{ .Nope }}",
				},
			},
		},
		{
			name:             "bad-skip-upload-tmpl",
			expectRunErrorIs: &template.Error{},
			nix: config.Nix{
				Name:       "doesnotmatter",
				SkipUpload: "{{ .Nope }}",
				Repository: config.RepoRef{
					Owner: "foo",
					Name:  "bar",
				},
			},
		},
		{
			name:             "bad-install-tmpl",
			expectRunErrorIs: &template.Error{},
			nix: config.Nix{
				Name:    "foo",
				Install: `{{.NoInstall}}`,
				Repository: config.RepoRef{
					Owner: "foo",
					Name:  "bar",
				},
			},
		},
		{
			name:             "bad-post-install-tmpl",
			expectRunErrorIs: &template.Error{},
			nix: config.Nix{
				Name:        "foo",
				PostInstall: `{{.NoPostInstall}}`,
				Repository: config.RepoRef{
					Owner: "foo",
					Name:  "bar",
				},
			},
		},
		{
			name:             "bad-path-tmpl",
			expectRunErrorIs: &template.Error{},
			nix: config.Nix{
				Name: "foo",
				Path: `{{.Foo}}/bar/foo.nix`,
				Repository: config.RepoRef{
					Owner: "foo",
					Name:  "bar",
				},
			},
		},
		{
			name:             "bad-release-url-tmpl",
			expectRunErrorIs: &template.Error{},
			nix: config.Nix{
				Name:        "foo",
				URLTemplate: "{{.BadURL}}",
				Repository: config.RepoRef{
					Owner: "foo",
					Name:  "bar",
				},
			},
		},
		{
			name:                 "skip-upload",
			expectPublishErrorIs: errSkipUpload,
			nix: config.Nix{
				Name:       "doesnotmatter",
				SkipUpload: "true",
				Repository: config.RepoRef{
					Owner: "foo",
					Name:  "bar",
				},
			},
		},
		{
			name:                 "skip-upload-auto",
			expectPublishErrorIs: errSkipUploadAuto,
			nix: config.Nix{
				Name:       "doesnotmatter",
				SkipUpload: "auto",
				Repository: config.RepoRef{
					Owner: "foo",
					Name:  "bar",
				},
			},
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			folder := t.TempDir()
			ctx := testctx.NewWithCfg(
				config.Project{
					Dist:        folder,
					ProjectName: "foo",
					Nix:         []config.Nix{tt.nix},
				},
				testctx.WithVersion("1.2.1"),
				testctx.WithCurrentTag("v1.2.1"),
				testctx.WithSemver(1, 2, 1, "rc1"),
			)
			createFakeArtifact := func(id, goos, goarch, goamd64, goarm string, extra map[string]any) {
				path := filepath.Join(folder, "dist/foo_"+goos+goarch+goamd64+goarm+".tar.gz")
				art := artifact.Artifact{
					Name:    "foo_" + goos + "_" + goarch + goamd64 + goarm + ".tar.gz",
					Path:    path,
					Goos:    goos,
					Goarch:  goarch,
					Goarm:   goarm,
					Goamd64: goamd64,
					Type:    artifact.UploadableArchive,
					Extra: map[string]interface{}{
						artifact.ExtraID:        id,
						artifact.ExtraFormat:    "tar.gz",
						artifact.ExtraBinaries:  []string{"foo"},
						artifact.ExtraWrappedIn: "",
					},
				}
				for k, v := range extra {
					art.Extra[k] = v
				}
				ctx.Artifacts.Add(&art)

				require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
				f, err := os.Create(path)
				require.NoError(t, err)
				require.NoError(t, f.Close())
			}

			createFakeArtifact("unibin-replaces", "darwin", "all", "", "", map[string]any{artifact.ExtraReplaces: true})
			createFakeArtifact("unibin", "darwin", "all", "", "", nil)
			for _, goos := range []string{"linux", "darwin", "windows"} {
				for _, goarch := range []string{"amd64", "arm64", "386", "arm"} {
					if goos+goarch == "darwin386" {
						continue
					}
					if goarch == "amd64" {
						createFakeArtifact("partial", goos, goarch, "v1", "", nil)
						createFakeArtifact("foo", goos, goarch, "v1", "", nil)
						createFakeArtifact("unibin", goos, goarch, "v1", "", nil)
						createFakeArtifact("unibin-replaces", goos, goarch, "v1", "", nil)
						createFakeArtifact("wrapped-in-dir", goos, goarch, "v1", "", map[string]any{artifact.ExtraWrappedIn: "./foo"})
					}
					if goarch == "arm" {
						if goos != "linux" {
							continue
						}
						createFakeArtifact("foo", goos, goarch, "", "6", nil)
						createFakeArtifact("foo", goos, goarch, "", "7", nil)
						continue
					}
					createFakeArtifact("foo", goos, goarch, "", "", nil)
					createFakeArtifact("unibin", goos, goarch, "", "", nil)
					createFakeArtifact("unibin-replaces", goos, goarch, "", "", nil)
					createFakeArtifact("wrapped-in-dir", goos, goarch, "", "", map[string]any{artifact.ExtraWrappedIn: "./foo"})
				}
			}

			client := client.NewMock()
			bpipe := NewBuild()
			ppipe := Pipe{
				fakeNixShaPrefetcher{
					"https://dummyhost/download/v1.2.1/foo_linux_amd64v1.tar.gz":  "sha1",
					"https://dummyhost/download/v1.2.1/foo_linux_arm64.tar.gz":    "sha2",
					"https://dummyhost/download/v1.2.1/foo_darwin_amd64v1.tar.gz": "sha3",
					"https://dummyhost/download/v1.2.1/foo_darwin_arm64.tar.gz":   "sha4",
					"https://dummyhost/download/v1.2.1/foo_darwin_all.tar.gz":     "sha5",
					"https://dummyhost/download/v1.2.1/foo_linux_arm6.tar.gz":     "sha6",
					"https://dummyhost/download/v1.2.1/foo_linux_arm7.tar.gz":     "sha7",
				},
			}

			// default
			require.NoError(t, bpipe.Default(ctx))

			// run
			if tt.expectRunErrorIs != nil {
				err := bpipe.runAll(ctx, client)
				require.ErrorAs(t, err, &tt.expectPublishErrorIs)
				return
			}
			require.NoError(t, bpipe.runAll(ctx, client))
			bts, err := os.ReadFile(ctx.Artifacts.Filter(artifact.ByType(artifact.Nixpkg)).Paths()[0])
			require.NoError(t, err)
			golden.RequireEqualExt(t, bts, "_build.nix")

			// publish
			if tt.expectPublishErrorIs != nil {
				err := ppipe.publishAll(ctx, client)
				require.ErrorAs(t, err, &tt.expectPublishErrorIs)
				return
			}
			require.NoError(t, ppipe.publishAll(ctx, client))
			require.True(t, client.CreatedFile)
			golden.RequireEqualExt(t, []byte(client.Content), "_publish.nix")
			require.NotContains(t, client.Content, strings.Repeat("0", 52))

			if tt.nix.Repository.PullRequest.Enabled {
				require.True(t, client.OpenedPullRequest)
			}
			if tt.nix.Path != "" {
				require.Equal(t, tt.nix.Path, client.Path)
			} else {
				if tt.nix.Name == "" {
					tt.nix.Name = "foo"
				}
				require.Equal(t, "pkgs/"+tt.nix.Name+"/default.nix", client.Path)
			}
		})
	}
}

func TestErrNoArchivesFound(t *testing.T) {
	require.EqualError(t, errNoArchivesFound{
		goamd64: "v1",
		ids:     []string{"foo", "bar"},
	}, "no archives found matching goos=[darwin linux] goarch=[amd64 arm arm64 386] goarm=[6 7] goamd64=v1 ids=[foo bar]")
}

func TestDependencies(t *testing.T) {
	require.Equal(t, []string{"nix-prefetch-url"}, Pipe{}.Dependencies(nil))
}

func TestBinInstallFormats(t *testing.T) {
	t.Run("no-deps", func(t *testing.T) {
		golden.RequireEqual(t, []byte(strings.Join(
			binInstallFormats(config.Nix{}),
			"\n",
		)))
	})
	t.Run("deps", func(t *testing.T) {
		golden.RequireEqual(t, []byte(strings.Join(
			binInstallFormats(config.Nix{
				Dependencies: []config.NixDependency{
					{Name: "fish"},
					{Name: "bash"},
					{Name: "zsh"},
				},
			}),
			"\n",
		)))
	})
	t.Run("linux-only-deps", func(t *testing.T) {
		golden.RequireEqual(t, []byte(strings.Join(
			binInstallFormats(config.Nix{
				Dependencies: []config.NixDependency{
					linuxDep("foo"),
					linuxDep("bar"),
				},
			}),
			"\n",
		)))
	})
	t.Run("darwin-only-deps", func(t *testing.T) {
		golden.RequireEqual(t, []byte(strings.Join(
			binInstallFormats(config.Nix{
				Dependencies: []config.NixDependency{
					darwinDep("foo"),
					darwinDep("bar"),
				},
			}),
			"\n",
		)))
	})
	t.Run("mixed-deps", func(t *testing.T) {
		golden.RequireEqual(t, []byte(strings.Join(
			binInstallFormats(config.Nix{
				Dependencies: []config.NixDependency{
					{Name: "fish"},
					linuxDep("foo"),
					darwinDep("bar"),
				},
			}),
			"\n",
		)))
	})
}

func darwinDep(s string) config.NixDependency {
	return config.NixDependency{
		Name: s,
		OS:   "darwin",
	}
}

func linuxDep(s string) config.NixDependency {
	return config.NixDependency{
		Name: s,
		OS:   "linux",
	}
}

type fakeNixShaPrefetcher map[string]string

func (m fakeNixShaPrefetcher) Prefetch(url string) (string, error) {
	return m[url], nil
}
func (m fakeNixShaPrefetcher) Available() bool { return true }
