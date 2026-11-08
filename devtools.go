package mygo

import (
	"context"
	"encoding/json/v2"

	"github.com/egoist/mygo/internal/platform"
)

// CallDevTools calls a method of the Chrome DevTools Protocol on the page
// and returns the JSON of its result, to do what the web's APIs do not:
// emulate a device, slow the network, read a response's body.
//
//	_, err := page.CallDevTools(ctx, "Emulation.setDeviceMetricsOverride", map[string]any{
//		"width": 390, "height": 844, "deviceScaleFactor": 3, "mobile": true,
//	})
//
// params is marshaled as the method's parameters, a JSON object; nil sends
// none. A method the protocol fails returns its message as the error.
// What a method changes lasts as the page navigates, for the life of its
// web view, as in a browser's tab.
//
// Only WebView2 (Windows) speaks the protocol, on every page, the
// inspector enabled or not: elsewhere CallDevTools returns an error that
// errors.Is(err, errors.ErrUnsupported) holds, as it does for a window
// showing native UI.
func (p *Page) CallDevTools(ctx context.Context, method string, params any) ([]byte, error) {
	if p.w.content {
		return nil, errNoPage
	}
	args := []byte("{}")
	if params != nil {
		var err error
		if args, err = json.Marshal(params); err != nil {
			return nil, err
		}
	}
	type result struct {
		raw string
		err error
	}
	// The cancellation, the page and the window closing may all answer;
	// the first one counts and the others must not block.
	ch := make(chan result, 3)
	stop := context.AfterFunc(ctx, func() { deliver(ch, result{err: ctx.Err()}) })
	defer stop()
	ran := false
	var off func()
	p.w.do(func(n platform.Window) {
		ran = true
		off = p.w.onClosed.add(func() { deliver(ch, result{err: errDestroyed}) }, false)
		n.CallDevToolsMethod(method, string(args), func(res string, err error) { deliver(ch, result{res, err}) })
	})
	if !ran {
		return nil, errDestroyed
	}
	r := await(ch)
	if off != nil {
		onMain(off)
	}
	if r.err != nil {
		return nil, r.err
	}
	return []byte(r.raw), nil
}

// OnDevToolsEvent is called with the JSON of the parameters of each event
// of the Chrome DevTools Protocol named event that the page raises, on the
// main thread, as other events are: a console message, a request sent or
// answered.
//
//	off, err := page.OnDevToolsEvent("Runtime.consoleAPICalled", func(params []byte) {
//		log.Printf("console: %s", params)
//	})
//
// The page raises a domain's events once the domain is enabled, as with
// page.CallDevTools(ctx, "Runtime.enable", nil), and from then on, as it
// navigates. Several listeners of one event all hear it; off stops this
// one. Only WebView2 (Windows) speaks the protocol: elsewhere the error is
// errors.ErrUnsupported, as for a window showing native UI.
func (p *Page) OnDevToolsEvent(event string, fn func(params []byte)) (off func(), err error) {
	if p.w.content {
		return nil, errNoPage
	}
	err = errDestroyed
	p.w.do(func(n platform.Window) {
		l := p.w.devEvents[event]
		if l == nil {
			l = &listeners[func([]byte)]{}
			if err = n.ListenDevToolsEvent(event, func(params string) {
				for _, fn := range l.snapshot() {
					fn([]byte(params))
				}
			}); err != nil {
				return
			}
			if p.w.devEvents == nil {
				p.w.devEvents = map[string]*listeners[func([]byte)]{}
			}
			p.w.devEvents[event] = l
		}
		err = nil
		remove := l.add(fn, false)
		// The last listener gone, the page stops telling the event.
		off = func() {
			onMain(func() {
				remove()
				if l.len() == 0 && p.w.devEvents[event] == l {
					delete(p.w.devEvents, event)
					if p.w.native != nil {
						p.w.native.ListenDevToolsEvent(event, nil)
					}
				}
			})
		}
	})
	return off, err
}
