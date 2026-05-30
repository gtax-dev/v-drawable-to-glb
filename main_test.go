package main

import (
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

const integrationEnv = "V_DRAWABLE_TO_GLB_INTEGRATION"

func TestBuildMultipartYdr(t *testing.T) {
	body, ct, err := buildMultipart(".ydr", "prop.ydr", []byte("ydrbytes"), "", nil, "phone", "high", "", -1, 0, 0, 0, "", "", "", false)
	if err != nil {
		t.Fatal(err)
	}
	if ct == "" || !strings.HasPrefix(ct, "multipart/form-data; boundary=") {
		t.Fatalf("unexpected content-type: %q", ct)
	}
	s := string(body)
	if !strings.Contains(s, `form-data; name="ydr"`) {
		t.Fatal("missing ydr field")
	}
	if !strings.Contains(s, "ydrbytes") {
		t.Fatal("missing ydr payload")
	}
	if !strings.Contains(s, `form-data; name="name"`) || !strings.Contains(s, "phone") {
		t.Fatal("missing name field")
	}
	if !strings.Contains(s, `form-data; name="lod"`) || !strings.Contains(s, "high") {
		t.Fatal("missing lod field")
	}
}

func TestBuildMultipartIncludesRotation(t *testing.T) {
	body, _, err := buildMultipart(".ydr", "a.ydr", []byte("x"), "", nil, "", "", "", -1, 0, 45, 0, "", "", "", false)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(body), `name="rotationY"`) || !strings.Contains(string(body), "45") {
		t.Fatalf("expected rotationY form field, got %q", string(body))
	}
}

func TestBuildMultipartYddWithIndex(t *testing.T) {
	body, _, err := buildMultipart(".ydd", "cloth.ydd", []byte("yddbytes"), "", nil, "", "", "", 2, 0, 0, 0, "", "", "", false)
	if err != nil {
		t.Fatal(err)
	}
	s := string(body)
	if !strings.Contains(s, `form-data; name="ydd"`) {
		t.Fatal("missing ydd field")
	}
	if !strings.Contains(s, `name="drawableIndex"`) || !strings.Contains(s, "2") {
		t.Fatal("missing drawableIndex")
	}
}

func TestBuildMultipartYft(t *testing.T) {
	body, _, err := buildMultipart(".yft", "taco.yft", []byte("yftbytes"), "taco.ytd", []byte("ytdbytes"), "taco", "", "", -1, 0, 0, 0, "taco_livery", "#ff0000", "gtax.dev", true)
	if err != nil {
		t.Fatal(err)
	}
	s := string(body)
	if !strings.Contains(s, `form-data; name="yft"`) || !strings.Contains(s, "yftbytes") {
		t.Fatal("missing yft field")
	}
	if !strings.Contains(s, `form-data; name="ytd"`) {
		t.Fatal("missing ytd field")
	}
	if !strings.Contains(s, `name="livery"`) || !strings.Contains(s, "taco_livery") {
		t.Fatal("missing livery field")
	}
	if !strings.Contains(s, `name="paint"`) || !strings.Contains(s, "#ff0000") {
		t.Fatal("missing paint field")
	}
	if !strings.Contains(s, `name="watermark"`) || !strings.Contains(s, "gtax.dev") {
		t.Fatal("missing watermark field")
	}
	if !strings.Contains(s, `name="skin"`) || !strings.Contains(s, "true") {
		t.Fatal("missing skin field")
	}
}

func TestBuildMultipartYftFieldsGatedToYft(t *testing.T) {
	// livery/paint/skin must only be sent for .yft input; watermark applies to all types.
	body, _, err := buildMultipart(".ydr", "a.ydr", []byte("x"), "", nil, "", "", "", -1, 0, 0, 0, "taco_livery", "#fff", "gtax.dev", true)
	if err != nil {
		t.Fatal(err)
	}
	s := string(body)
	if strings.Contains(s, `name="livery"`) || strings.Contains(s, `name="paint"`) || strings.Contains(s, `name="skin"`) {
		t.Fatalf("yft-only fields leaked into a .ydr request: %q", s)
	}
	if !strings.Contains(s, `name="watermark"`) || !strings.Contains(s, "gtax.dev") {
		t.Fatal("watermark should be sent for .ydr input")
	}
}

func TestReadErrorBodyJSON(t *testing.T) {
	resp := &http.Response{
		Body: io.NopCloser(strings.NewReader(`{"error":"quota exceeded"}`)),
	}
	msg := readErrorBody(resp)
	if msg != "quota exceeded" {
		t.Fatalf("got %q", msg)
	}
}

func TestReadErrorBodyPlain(t *testing.T) {
	resp := &http.Response{
		Body: io.NopCloser(strings.NewReader("plain text")),
	}
	msg := readErrorBody(resp)
	if msg != "plain text" {
		t.Fatalf("got %q", msg)
	}
}

func TestIntegrationYdrExampleLive(t *testing.T) {
	skipUnlessIntegration(t)

	ydrPath := exampleAsset(t, "prop_phone_ing_03.ydr")
	ytdPath := exampleAsset(t, "cellphone_badger.ytd")
	ydrData, err := os.ReadFile(ydrPath)
	if err != nil {
		t.Fatal(err)
	}
	ytdData, err := os.ReadFile(ytdPath)
	if err != nil {
		t.Fatal(err)
	}

	body, ct, err := buildMultipart(".ydr", ydrPath, ydrData, ytdPath, ytdData, "phone", "high", "", -1, 0, 0, 0, "", "", "", false)
	if err != nil {
		t.Fatal(err)
	}

	client := &http.Client{Timeout: 15 * time.Minute}
	key := strings.TrimSpace(os.Getenv(apiKeyEnv))
	resp, err := postConvert(defaultPublicAPI, "/convert/ydr-to-glb", body, ct, key, client)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	assertIntegrationGLB(t, resp)
}

func TestIntegrationYddExampleLive(t *testing.T) {
	skipUnlessIntegration(t)

	yddPath := exampleAsset(t, "lowr_057_u.ydd")
	ytdPath := exampleAsset(t, "lowr_diff_057_a_uni.ytd")
	yddData, err := os.ReadFile(yddPath)
	if err != nil {
		t.Fatal(err)
	}
	ytdData, err := os.ReadFile(ytdPath)
	if err != nil {
		t.Fatal(err)
	}

	body, ct, err := buildMultipart(".ydd", yddPath, yddData, ytdPath, ytdData, "lowr", "", "", 0, 0, 0, 0, "", "", "", false)
	if err != nil {
		t.Fatal(err)
	}

	client := &http.Client{Timeout: 15 * time.Minute}
	key := strings.TrimSpace(os.Getenv(apiKeyEnv))
	resp, err := postConvert(defaultPublicAPI, "/convert/ydd-to-glb", body, ct, key, client)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	assertIntegrationGLB(t, resp)
}

func TestIntegrationYftExampleLive(t *testing.T) {
	skipUnlessIntegration(t)

	yftPath := exampleAsset(t, "taco.yft")
	ytdPath := exampleAsset(t, "taco.ytd")
	yftData, err := os.ReadFile(yftPath)
	if err != nil {
		t.Fatal(err)
	}
	ytdData, err := os.ReadFile(ytdPath)
	if err != nil {
		t.Fatal(err)
	}

	body, ct, err := buildMultipart(".yft", yftPath, yftData, ytdPath, ytdData, "taco", "", "", -1, 0, 0, 0, "taco_livery", "#ff0000", "", false)
	if err != nil {
		t.Fatal(err)
	}

	client := &http.Client{Timeout: 15 * time.Minute}
	key := strings.TrimSpace(os.Getenv(apiKeyEnv))
	resp, err := postConvert(defaultPublicAPI, "/convert/yft-to-glb", body, ct, key, client)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	assertIntegrationGLB(t, resp)
}

func skipUnlessIntegration(t *testing.T) {
	t.Helper()
	if testing.Short() {
		t.Skip("skipping live API tests in -short mode")
	}
	if os.Getenv(integrationEnv) != "1" {
		t.Skipf("set %s=1 to run live API tests against example assets", integrationEnv)
	}
}

func exampleAsset(t *testing.T, name string) string {
	t.Helper()
	p := filepath.Join("example", name)
	if _, err := os.Stat(p); err != nil {
		t.Skipf("missing example/%s: %v", name, err)
	}
	return p
}

func assertIntegrationGLB(t *testing.T, resp *http.Response) {
	t.Helper()
	switch resp.StatusCode {
	case http.StatusOK:
		// ok
	case http.StatusTooManyRequests, http.StatusServiceUnavailable:
		msg := readErrorBody(resp)
		t.Skipf("live API: %s: %s", resp.Status, msg)
	default:
		msg := readErrorBody(resp)
		t.Fatalf("live API: %s: %s", resp.Status, msg)
	}

	ct := resp.Header.Get("Content-Type")
	if !strings.HasPrefix(ct, "model/gltf-binary") {
		t.Fatalf("unexpected Content-Type %q", ct)
	}

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	if len(data) < 12 {
		t.Fatalf("GLB too small: %d bytes", len(data))
	}
	if string(data[0:4]) != "glTF" {
		t.Fatalf("not a GLB container (want glTF magic), got %q", data[0:4])
	}
}
