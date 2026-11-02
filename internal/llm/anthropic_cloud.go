package llm

// Claude through the clouds. Both serve the Messages API and its stream; only the address and the
// credentials differ. Neither has a model listing, so the models are a table.
//
//   Amazon Bedrock (https://platform.claude.com/docs/en/build-with-claude/claude-in-amazon-bedrock, read
//   2026-11-02): https://bedrock-mantle.{region}.api.aws/anthropic/v1/messages, ids prefixed "anthropic.",
//   SigV4 with service bedrock-mantle, or a bearer token in x-api-key. The connection key is that token,
//   or ACCESS_KEY:SECRET_KEY[:SESSION_TOKEN].
//
//   Google Cloud (https://platform.claude.com/docs/en/build-with-claude/claude-models-on-google-cloud/google-managed,
//   read 2026-11-02): {base}/publishers/anthropic/models/{id}:streamRawPredict, where the connection's base
//   is https://aiplatform.googleapis.com/v1/projects/{project}/locations/global; the model goes in the URL,
//   anthropic_version in the body, a bearer token from the machine's Google credentials in Authorization.

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"infuse/internal/platform/process"
	"net/http"
	"net/url"
	"os/exec"
	"strings"
	"sync"
	"time"
)

const (
	bedrockDefaultBase = "https://bedrock-mantle.us-east-1.api.aws/anthropic"
	bedrockService     = "bedrock-mantle"
	vertexVersion      = "vertex-2023-10-16"
)

var bedrockModels = []string{
	"anthropic.claude-fable-5-1", "anthropic.claude-fable-5", "anthropic.claude-opus-5-5", "anthropic.claude-opus-5",
	"anthropic.claude-opus-4-8", "anthropic.claude-opus-4-7", "anthropic.claude-sonnet-5-5", "anthropic.claude-sonnet-5",
	"anthropic.claude-haiku-5-5", "anthropic.claude-haiku-4-5",
}

var vertexModels = []string{
	"claude-fable-5-1", "claude-fable-5", "claude-opus-5-5", "claude-opus-5", "claude-opus-4-8", "claude-opus-4-7",
	"claude-opus-4-6", "claude-sonnet-5-5", "claude-sonnet-5", "claude-sonnet-4-6", "claude-haiku-5-5",
}

func init() {
	RegisterProvider("bedrock", func(cfg ProviderConfig) Provider {
		return &anthropicProvider{name: "bedrock", access: bedrockAccess{key: cfg.APIKey, base: orDefault(cfg.BaseURL, bedrockDefaultBase)}, client: httpClient()}
	})
	RegisterProvider("vertex", func(cfg ProviderConfig) Provider {
		return &anthropicProvider{name: "vertex", access: &vertexAccess{key: cfg.APIKey, base: strings.TrimRight(cfg.BaseURL, "/")}, client: httpClient()}
	})
}

// ---------------------------------------------------------------- Amazon Bedrock

type bedrockAccess struct{ key, base string }

func (a bedrockAccess) url(string) string    { return a.base + "/v1/messages" }
func (a bedrockAccess) adapt(map[string]any) {}

func (a bedrockAccess) credentials(context.Context) (map[string]string, signFunc, error) {
	headers := map[string]string{"anthropic-version": anthropicVersion}
	if a.key == "" {
		return nil, nil, errors.New("Bedrock: enter a bearer token or ACCESS_KEY:SECRET_KEY as the connection key")
	}
	creds, isPair := parseAWSKey(a.key)
	if !isPair {
		headers["x-api-key"] = a.key
		return headers, nil, nil
	}
	region := bedrockRegion(a.base)
	return headers, func(r *http.Request, body []byte) { signV4(r, body, creds, region, bedrockService, time.Now()) }, nil
}

// parseAWSKey reads ACCESS_KEY:SECRET_KEY[:SESSION_TOKEN]; anything else is a bearer token.
func parseAWSKey(key string) (awsCredentials, bool) {
	p := strings.SplitN(key, ":", 3)
	if len(p) < 2 || !strings.HasPrefix(p[0], "A") || p[1] == "" {
		return awsCredentials{}, false
	}
	c := awsCredentials{access: p[0], secret: p[1]}
	if len(p) == 3 {
		c.session = p[2]
	}
	return c, true
}

// bedrockRegion comes from the endpoint: bedrock-mantle.{region}.api.aws.
func bedrockRegion(base string) string {
	u, err := url.Parse(base)
	if err == nil {
		if p := strings.Split(u.Hostname(), "."); len(p) > 2 && p[0] == bedrockService {
			return p[1]
		}
	}
	return "us-east-1"
}

// ---------------------------------------------------------------- Google Cloud

// vertexAccess keeps the machine's token until it is about to expire.
type vertexAccess struct {
	key, base string
	mu        sync.Mutex
	token     string
	until     time.Time
}

func (a *vertexAccess) url(model string) string {
	return a.base + "/publishers/anthropic/models/" + url.PathEscape(model) + ":streamRawPredict"
}

func (a *vertexAccess) adapt(body map[string]any) {
	delete(body, "model")
	body["anthropic_version"] = vertexVersion
}

func (a *vertexAccess) credentials(ctx context.Context) (map[string]string, signFunc, error) {
	if a.base == "" {
		return nil, nil, errors.New("Google Cloud: enter the endpoint, https://aiplatform.googleapis.com/v1/projects/PROJECT/locations/global")
	}
	token, err := a.bearer(ctx)
	if err != nil {
		return nil, nil, err
	}
	return map[string]string{"authorization": "Bearer " + token}, nil, nil
}

func (a *vertexAccess) bearer(ctx context.Context) (string, error) {
	if a.key != "" {
		return a.key, nil
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.token != "" && time.Now().Before(a.until) {
		return a.token, nil
	}
	token, life, err := googleToken(ctx)
	if err != nil {
		return "", err
	}
	a.token, a.until = token, time.Now().Add(life-time.Minute)
	return token, nil
}

// Where a Google token comes from; variables so that tests use fakes, never the machine's account.
var (
	gcloudCommand = func(ctx context.Context) ([]byte, error) {
		cmd := exec.CommandContext(ctx, "gcloud", "auth", "application-default", "print-access-token")
		process.NoConsoleWindow(cmd)
		return cmd.Output()
	}
	metadataTokenURL = "http://metadata.google.internal/computeMetadata/v1/instance/service-accounts/default/token"
)

// googleToken takes the token from gcloud's Application Default Credentials, else from the metadata
// server of the machine it runs on.
func googleToken(ctx context.Context) (string, time.Duration, error) {
	c, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	if out, err := gcloudCommand(c); err == nil && strings.TrimSpace(string(out)) != "" {
		return strings.TrimSpace(string(out)), 30 * time.Minute, nil // gcloud does not say how long it lasts
	}
	token, life, err := metadataToken(ctx)
	if err != nil {
		return "", 0, fmt.Errorf("Google Cloud: no credentials (run gcloud auth application-default login): %w", err)
	}
	return token, life, nil
}

func metadataToken(ctx context.Context) (string, time.Duration, error) {
	client := &http.Client{Timeout: 3 * time.Second}
	b, err := fetchJSON(ctx, client, "metadata", metadataTokenURL, map[string]string{"Metadata-Flavor": "Google"})
	if err != nil {
		return "", 0, err
	}
	var r struct {
		AccessToken string `json:"access_token"`
		ExpiresIn   int    `json:"expires_in"`
	}
	if err := json.Unmarshal(b, &r); err != nil || r.AccessToken == "" {
		return "", 0, errors.New("the metadata server gave no token")
	}
	return r.AccessToken, time.Duration(r.ExpiresIn) * time.Second, nil
}

// cloudModels proves the credentials are there, without a request to the model.
func cloudModels(ctx context.Context, name string, cfg ProviderConfig) ([]string, error) {
	mk := providers[name](cfg).(*anthropicProvider)
	if _, _, err := mk.access.credentials(ctx); err != nil {
		return nil, err
	}
	if name == "vertex" {
		return append([]string{}, vertexModels...), nil
	}
	return append([]string{}, bedrockModels...), nil
}
