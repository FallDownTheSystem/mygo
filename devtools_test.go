//go:build !js

package mygo

import (
	"context"
	"errors"
	"testing"

	"github.com/egoist/mygo/internal/fake"
)

// CallDevTools sends the method and its parameters as JSON, and returns
// the protocol's answer, or its failure; a page that speaks no protocol
// says it is unsupported.
func TestCallDevTools(t *testing.T) {
	show := true
	_, v, _, fv := webViewWindow(t, &show)
	page := v.Page()
	fv.DevToolsMethod = func(method, params string) (string, error) {
		if method == "Network.getResponseBody" {
			return "", errors.New("No resource with given identifier found")
		}
		return `{"echo":` + params + `}`, nil
	}
	res, err := page.CallDevTools(context.Background(), "Emulation.setDeviceMetricsOverride", struct {
		Width  int  `json:"width"`
		Mobile bool `json:"mobile"`
	}{390, true})
	if err != nil || string(res) != `{"echo":{"width":390,"mobile":true}}` {
		t.Errorf("CallDevTools = %s, %v", res, err)
	}
	if res, err := page.CallDevTools(context.Background(), "Runtime.enable", nil); err != nil || string(res) != `{"echo":{}}` {
		t.Errorf("CallDevTools without params = %s, %v", res, err)
	}
	if _, err := page.CallDevTools(context.Background(), "Network.getResponseBody", map[string]string{"requestId": "1"}); err == nil || err.Error() != "No resource with given identifier found" {
		t.Errorf("a failed method: %v", err)
	}
	if got := fv.DevToolsCalls; len(got) != 3 || got[0].Method != "Emulation.setDeviceMetricsOverride" || got[1].Params != "{}" {
		t.Errorf("calls %+v", got)
	}

	fv.NoDevTools = true
	if _, err := page.CallDevTools(context.Background(), "Runtime.enable", nil); !errors.Is(err, errors.ErrUnsupported) {
		t.Errorf("no protocol: %v", err)
	}
	if _, err := page.OnDevToolsEvent("Runtime.consoleAPICalled", func([]byte) {}); !errors.Is(err, errors.ErrUnsupported) {
		t.Errorf("no protocol's events: %v", err)
	}
}

// Every listener of an event hears it, and only its own event; the page
// tells the event until its last listener goes.
func TestOnDevToolsEvent(t *testing.T) {
	w, fw := testWindow(t, WindowOptions{})
	var a, b, other []string
	offA, err := w.Page().OnDevToolsEvent("Runtime.consoleAPICalled", func(p []byte) { a = append(a, string(p)) })
	if err != nil {
		t.Fatal(err)
	}
	offB, _ := w.Page().OnDevToolsEvent("Runtime.consoleAPICalled", func(p []byte) { b = append(b, string(p)) })
	offOther, _ := w.Page().OnDevToolsEvent("Network.loadingFailed", func(p []byte) { other = append(other, string(p)) })
	emit := func(fw *fake.Window, event, params string) bool {
		var ok bool
		onMain(func() { ok = fw.EmitDevToolsEvent(event, params) })
		return ok
	}
	emit(fw, "Runtime.consoleAPICalled", `{"type":"log"}`)
	offA()
	offA() // twice does no more
	emit(fw, "Runtime.consoleAPICalled", `{"type":"error"}`)
	emit(fw, "Network.loadingFailed", `{"requestId":"7"}`)
	if len(a) != 1 || len(b) != 2 || b[1] != `{"type":"error"}` || len(other) != 1 {
		t.Errorf("heard %q, %q and %q", a, b, other)
	}
	offB()
	if fw.DevToolsListening("Runtime.consoleAPICalled") || !fw.DevToolsListening("Network.loadingFailed") {
		t.Error("the page tells an event none listens to, or stopped one listened to")
	}
	offOther()
	// A listener added again has the page tell it again.
	if _, err := w.Page().OnDevToolsEvent("Runtime.consoleAPICalled", func(p []byte) { a = append(a, string(p)) }); err != nil || !emit(fw, "Runtime.consoleAPICalled", `{}`) || len(a) != 2 {
		t.Errorf("listening again: %v, %q", err, a)
	}
}
