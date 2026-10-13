package flexera

import (
	"fmt"
	"net/http"
	"regexp"
	"runtime"
	"runtime/debug"
	"strings"
	"sync"
)

const (
	modulePath       = "github.com/flexera-public/unified-go-client"
	userAgentProduct = "flexera-unified-go-client"
)

// Version may be set at link time to stamp the client version, e.g.
//
//	go build -ldflags "-X github.com/flexera-public/unified-go-client.Version=v1.2.3"
//
// When empty, the version is derived from the Go build info (module version
// when used as a dependency, VCS revision when built from a checkout).
var Version = ""

var (
	defaultUAOnce sync.Once
	defaultUA     string
)

// DefaultUserAgent returns the User-Agent sent by default, for example:
//
//	flexera-unified-go-client/v0.3.1 (abc1234; go1.24.5; darwin/arm64)
//
// The parenthesized metadata lists the VCS revision (with "dirty" if the
// working tree was modified, when known) followed by the Go toolchain and
// platform.
func DefaultUserAgent() string {
	defaultUAOnce.Do(func() {
		bi, _ := debug.ReadBuildInfo()
		defaultUA = buildUserAgent(Version, bi, runtime.Version(), runtime.GOOS, runtime.GOARCH)
	})
	return defaultUA
}

// pseudoVersionRev extracts the commit hash from a Go pseudo-version such as
// v0.0.0-20240102150405-abcdef123456.
var pseudoVersionRev = regexp.MustCompile(`-([0-9a-f]{12})(?:\+dirty)?$`)

func buildUserAgent(version string, bi *debug.BuildInfo, goVersion, goos, goarch string) string {
	var rev string
	dirty := false
	modVersion := ""

	if bi != nil {
		if bi.Main.Path == modulePath {
			modVersion = bi.Main.Version
		}
		for _, d := range bi.Deps {
			if d.Path == modulePath {
				if d.Replace != nil {
					modVersion = d.Replace.Version
				} else {
					modVersion = d.Version
				}
			}
		}
		// VCS settings describe the main module; only trust them when that
		// is this module.
		if bi.Main.Path == modulePath {
			for _, s := range bi.Settings {
				switch s.Key {
				case "vcs.revision":
					rev = s.Value
				case "vcs.modified":
					dirty = s.Value == "true"
				}
			}
		}
	}

	if modVersion == "(devel)" {
		modVersion = ""
	}
	if version == "" {
		version = modVersion
	}
	if version == "" {
		version = "devel"
	}
	if rev == "" {
		if m := pseudoVersionRev.FindStringSubmatch(version); m != nil {
			rev = m[1]
		}
	}
	if strings.HasSuffix(version, "+dirty") {
		dirty = true
	}
	if len(rev) > 12 {
		rev = rev[:12]
	}

	meta := make([]string, 0, 4)
	if rev != "" {
		meta = append(meta, rev)
	}
	if dirty {
		meta = append(meta, "dirty")
	}
	meta = append(meta, goVersion, goos+"/"+goarch)
	return fmt.Sprintf("%s/%s (%s)", userAgentProduct, version, strings.Join(meta, "; "))
}

// userAgentDoer sets the User-Agent header on requests that do not already
// carry one, so explicit per-request values always win.
type userAgentDoer struct {
	next HttpRequestDoer
	ua   string
}

// NewUserAgentDoer wraps next so requests without a User-Agent header get ua.
// An empty ua uses DefaultUserAgent(). If next is nil, http.DefaultClient is used.
func NewUserAgentDoer(next HttpRequestDoer, ua string) HttpRequestDoer {
	if next == nil {
		next = http.DefaultClient
	}
	if strings.TrimSpace(ua) == "" {
		ua = DefaultUserAgent()
	}
	return &userAgentDoer{next: next, ua: ua}
}

func (d *userAgentDoer) Do(req *http.Request) (*http.Response, error) {
	if req.Header.Get("User-Agent") == "" {
		req = req.Clone(req.Context())
		req.Header.Set("User-Agent", d.ua)
	}
	return d.next.Do(req)
}

// WithDefaultUserAgent wraps the client's HTTP doer so requests without a
// User-Agent get DefaultUserAgent(). Clients built by
// NewClientWithResponsesForAuth and AuthHelper already include this; use it
// only with the bare NewClient / NewClientWithResponses constructors.
func WithDefaultUserAgent() ClientOption {
	return WithUserAgent("")
}

// WithUserAgent replaces the default User-Agent with ua. Place it after any
// option that installs the HTTP doer. A User-Agent set on an individual
// request (WithRequestEditorFn or per-call editors) still takes precedence.
// An empty ua is equivalent to WithDefaultUserAgent.
func WithUserAgent(ua string) ClientOption {
	return func(c *Client) error {
		c.Client = NewUserAgentDoer(c.Client, ua)
		return nil
	}
}

// WithUserAgentProduct prefixes the default User-Agent with an application
// product token, e.g. WithUserAgentProduct("flexera-cli", "1.2.3") yields
// "flexera-cli/1.2.3 flexera-unified-go-client/v0.3.1 (...)". This lets an
// embedding application identify itself while keeping client version info.
func WithUserAgentProduct(name, version string) ClientOption {
	name = strings.TrimSpace(name)
	if name == "" {
		return WithDefaultUserAgent()
	}
	if v := strings.TrimSpace(version); v != "" {
		name += "/" + v
	}
	return WithUserAgent(name + " " + DefaultUserAgent())
}
