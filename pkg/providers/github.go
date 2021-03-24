package providers

import (
	"context"
	"crypto/sha256"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"strings"

	"github.com/apex/log"
	"github.com/google/go-github/v31/github"
	"github.com/marcosnils/bin/pkg/assets"
	"golang.org/x/oauth2"
)

type gitHub struct {
	url    *url.URL
	client *github.Client
	owner  string
	repo   string
	tag    string
}

func (g *gitHub) Fetch() (*File, error) {
	var release *github.RepositoryRelease

	// If we have a tag, let's fetch from there
	var err error
	if len(g.tag) > 0 {
		log.Infof("Getting %s release for %s/%s", g.tag, g.owner, g.repo)
		release, _, err = g.client.Repositories.GetReleaseByTag(context.TODO(), g.owner, g.repo, g.tag)
	} else {
		// TODO handle case when repo doesn't have releases?
		log.Infof("Getting latest release for %s/%s", g.owner, g.repo)
		release, err = g.getAnyLatestRelease(context.TODO())
	}

	if err != nil {
		return nil, err
	}

	candidates := []*assets.Asset{}
	for _, a := range release.Assets {
		candidates = append(candidates, &assets.Asset{Name: a.GetName(), URL: a.GetBrowserDownloadURL()})
	}
	gf, err := assets.FilterAssets(g.repo, candidates)
	if err != nil {
		return nil, err
	}

	name, outputFile, err := assets.ProcessURL(gf)
	if err != nil {
		return nil, err
	}

	version := release.GetTagName()

	// TODO calculate file hash. Not sure if we can / should do it here
	// since we don't want to read the file unnecesarily. Additionally, sometimes
	// releases have .sha256 files, so it'd be nice to check for those also
	f := &File{Data: outputFile, Name: assets.SanitizeName(name, version), Hash: sha256.New(), Version: version}

	return f, nil
}

// GetLatestVersion checks the latest repo release and
// returns the corresponding name and url to fetch the version
func (g *gitHub) GetLatestVersion() (string, string, error) {
	log.Debugf("Getting latest release for %s/%s", g.owner, g.repo)
	release, err := g.getAnyLatestRelease(context.TODO())
	if err != nil {
		return "", "", err
	}

	return release.GetTagName(), release.GetHTMLURL(), nil
}

func (g *gitHub) getAnyLatestRelease(ctx context.Context) (*github.RepositoryRelease, error) {
	release, _, err := g.client.Repositories.GetLatestRelease(ctx, g.owner, g.repo)
	if err != nil {
		// If the error is that no latest release was found, it could be that there are only pre-releases
		if ghErrResp, ok := err.(*github.ErrorResponse); ok && ghErrResp.Response.StatusCode == http.StatusNotFound {
			// Get the first release returned by ListReleases
			releases, _, listErr := g.client.Repositories.ListReleases(ctx, g.owner, g.repo, &github.ListOptions{PerPage: 1})
			if listErr != nil {
				return nil, listErr
			}
			if len(releases) > 0 {
				return releases[0], nil
			}
		}

		// Return original 404/StatusNotFound error from GetLatestRelease
		return nil, err
	}
	return release, err
}

func (g *gitHub) GetID() string {
	return "github"
}

func newGitHub(u *url.URL) (Provider, error) {
	s := strings.Split(u.Path, "/")
	if len(s) < 2 {
		return nil, fmt.Errorf("Error parsing Github URL %s, can't find owner and repo", u.String())
	}

	// it's a specific releases URL
	var tag string
	if strings.Contains(u.Path, "/releases/") {
		// For release and download URL's, the
		// path is usually /releases/tag/v0.1
		// or /releases/download/v0.1.
		ps := strings.Split(u.Path, "/")
		for i, p := range ps {
			if p == "releases" {
				tag = strings.Join(ps[i+2:], "/")
			}
		}

	}

	token := os.Getenv("GITHUB_AUTH_TOKEN")
	var tc *http.Client
	if token != "" {
		tc = oauth2.NewClient(context.Background(), oauth2.StaticTokenSource(
			&oauth2.Token{AccessToken: token},
		))
	}
	client := github.NewClient(tc)
	return &gitHub{url: u, client: client, owner: s[1], repo: s[2], tag: tag}, nil
}
