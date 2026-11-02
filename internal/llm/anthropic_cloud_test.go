package llm

// Bedrock and Google Cloud against fake servers: the address, the credentials and the request.

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"
	"time"
)

// The two requests of the AWS Signature Version 4 test suite (get-vanilla, post-vanilla).
func TestSigV4AWSSuite(t *testing.T) {
	creds := awsCredentials{access: "AKIDEXAMPLE", secret: "wJalrXUtnFEMI/K7MDENG+bPxRfiCYEXAMPLEKEY"}
	at := time.Date(2015, 8, 30, 12, 36, 0, 0, time.UTC)
	for method, sig := range map[string]string{
		"GET":  "5fa00fa31553b73ebf1942676e86291e8372ff2a2260956d9b8aae1d763fbf31",
		"POST": "5da7c1a2acd57cee7505fc6676e4e544621c30862966e37dddb68e92efbe5d6b",
	} {
		r, _ := http.NewRequest(method, "https://example.amazonaws.com/", nil)
		signV4(r, nil, creds, "us-east-1", "service", at)
		want := "AWS4-HMAC-SHA256 Credential=AKIDEXAMPLE/20150830/us-east-1/service/aws4_request, SignedHeaders=host;x-amz-date, Signature=" + sig
		if got := r.Header.Get("Authorization"); got != want {
			t.Fatalf("%s\n got %s\nwant %s", method, got, want)
		}
	}
}

func TestSigV4SignsTheSessionTokenAndBody(t *testing.T) {
	sign := func(body string) string {
		r, _ := http.NewRequest("POST", "https://bedrock-mantle.eu-west-1.api.aws/anthropic/v1/messages", strings.NewReader(body))
		r.Header.Set("content-type", "application/json")
		signV4(r, []byte(body), awsCredentials{"AKID", "secret", "tok"}, "eu-west-1", bedrockService, time.Unix(1, 0))
		if r.Header.Get("x-amz-security-token") != "tok" {
			t.Fatal("no session token")
		}
		return r.Header.Get("Authorization")
	}
	a, b := sign("one"), sign("two")
	if a == b || !strings.Contains(a, "SignedHeaders=content-type;host;x-amz-date;x-amz-security-token") || !strings.Contains(a, "/eu-west-1/bedrock-mantle/aws4_request") {
		t.Fatalf("%s\n%s", a, b)
	}
}

func TestBedrockSignedRequest(t *testing.T) {
	f := &fakeMessages{replies: []func(http.ResponseWriter){fullReply}}
	srv := httptest.NewServer(f)
	defer srv.Close()
	p, err := newProvider("bedrock", ProviderConfig{APIKey: "AKIDTEST:secret:session", BaseURL: srv.URL + "/anthropic"})
	if err != nil {
		t.Fatal(err)
	}
	res, err := p.Stream(context.Background(), ModelRequest{Model: "anthropic.claude-opus-5-5", Messages: convo[:1], MaxTokens: 16000, Thinking: 4000, Effort: "medium"}, func(StreamEvent) {})
	if err != nil || res.StopReason != "tool_use" {
		t.Fatalf("%v %+v", err, res)
	}
	h := f.headers[0]
	if f.paths[0] != "/anthropic/v1/messages" || h.Get("x-api-key") != "" || h.Get("anthropic-version") != "2023-06-01" || h.Get("x-amz-security-token") != "session" {
		t.Fatalf("%s %v", f.paths[0], h)
	}
	if !regexp.MustCompile(`^AWS4-HMAC-SHA256 Credential=AKIDTEST/\d{8}/us-east-1/bedrock-mantle/aws4_request, SignedHeaders=\S+, Signature=[0-9a-f]{64}$`).MatchString(h.Get("Authorization")) {
		t.Fatalf("authorization: %s", h.Get("Authorization"))
	}
	sameJSON(t, f.bodies[0], `{"model":"anthropic.claude-opus-5-5","max_tokens":16000,"stream":true,"thinking":{"type":"adaptive","display":"summarized"},"output_config":{"effort":"medium"},"messages":[{"role":"user","content":[{"type":"text","text":"hi"}]}]}`)
}

func TestBedrockBearerToken(t *testing.T) {
	f := &fakeMessages{replies: []func(http.ResponseWriter){fullReply}}
	srv := httptest.NewServer(f)
	defer srv.Close()
	p, _ := newProvider("bedrock", ProviderConfig{APIKey: "bedrock-api-key-abc", BaseURL: srv.URL})
	if _, err := p.Stream(context.Background(), ModelRequest{Model: "anthropic.claude-sonnet-5-5", Messages: convo[:1], MaxTokens: 100}, func(StreamEvent) {}); err != nil {
		t.Fatal(err)
	}
	if h := f.headers[0]; h.Get("x-api-key") != "bedrock-api-key-abc" || h.Get("Authorization") != "" {
		t.Fatalf("%v", h)
	}
}

func TestBedrockRegionFromEndpoint(t *testing.T) {
	if r := bedrockRegion("https://bedrock-mantle.eu-west-1.api.aws/anthropic"); r != "eu-west-1" {
		t.Fatal(r)
	}
	if _, pair := parseAWSKey("bedrock-api-key-abc"); pair {
		t.Fatal("a token read as a key pair")
	}
}

func TestVertexRequestAndMetadataToken(t *testing.T) {
	calls := 0
	meta := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if r.Header.Get("Metadata-Flavor") != "Google" {
			t.Error("metadata server needs its header")
		}
		_, _ = io.WriteString(w, `{"access_token":"tok-meta","expires_in":3600}`)
	}))
	defer meta.Close()
	prevURL, prevCmd := metadataTokenURL, gcloudCommand
	metadataTokenURL = meta.URL
	gcloudCommand = func(context.Context) ([]byte, error) { return nil, errors.New("no gcloud") }
	defer func() { metadataTokenURL, gcloudCommand = prevURL, prevCmd }()

	f := &fakeMessages{replies: []func(http.ResponseWriter){fullReply}}
	srv := httptest.NewServer(f)
	defer srv.Close()
	p, _ := newProvider("vertex", ProviderConfig{BaseURL: srv.URL + "/v1/projects/p/locations/global"})
	req := ModelRequest{Model: "claude-opus-5-5", Messages: convo[:1], MaxTokens: 16000, Thinking: 4000, Effort: "medium"}
	for i := 0; i < 2; i++ {
		if _, err := p.Stream(context.Background(), req, func(StreamEvent) {}); err != nil {
			t.Fatal(err)
		}
	}
	if calls != 1 {
		t.Fatalf("the token must be kept until it expires: %d fetches", calls)
	}
	if f.paths[0] != "/v1/projects/p/locations/global/publishers/anthropic/models/claude-opus-5-5:streamRawPredict" || f.headers[0].Get("Authorization") != "Bearer tok-meta" {
		t.Fatalf("%s %v", f.paths[0], f.headers[0])
	}
	sameJSON(t, f.bodies[0], `{"anthropic_version":"vertex-2023-10-16","max_tokens":16000,"stream":true,"thinking":{"type":"adaptive","display":"summarized"},"output_config":{"effort":"medium"},"messages":[{"role":"user","content":[{"type":"text","text":"hi"}]}]}`)
}

func TestVertexGcloudTokenFirst(t *testing.T) {
	prev := gcloudCommand
	gcloudCommand = func(context.Context) ([]byte, error) { return []byte("tok-gcloud\n"), nil }
	defer func() { gcloudCommand = prev }()
	token, _, err := googleToken(context.Background())
	if err != nil || token != "tok-gcloud" {
		t.Fatalf("%q %v", token, err)
	}
}

func TestCloudConnectionsListWithoutARequest(t *testing.T) {
	if _, err := ListModels(context.Background(), "bedrock", ProviderConfig{}); err == nil {
		t.Fatal("bedrock without credentials accepted")
	}
	names, err := ListModels(context.Background(), "bedrock", ProviderConfig{APIKey: "token"})
	if err != nil || len(names) != len(bedrockModels) || !strings.HasPrefix(names[0], "anthropic.") {
		t.Fatalf("%v %v", names, err)
	}
	if _, err := ListModels(context.Background(), "vertex", ProviderConfig{}); err == nil || !strings.Contains(err.Error(), "endpoint") {
		t.Fatalf("vertex without an endpoint: %v", err)
	}
	names, err = ListModels(context.Background(), "vertex", ProviderConfig{APIKey: "t", BaseURL: "https://x/v1/projects/p/locations/global"})
	if err != nil || len(names) != len(vertexModels) {
		t.Fatalf("%v %v", names, err)
	}
}
