package sentryfiber

import (
	"bytes"
	"context"
	"fmt"
	"io/ioutil"
	"net"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/getsentry/sentry-go"
	"github.com/gofiber/fiber/v2"
	"github.com/valyala/fasthttp"
)

const valuesKey = "sentry"

type handler struct {
	repanic         bool
	waitForDelivery bool
	timeout         time.Duration
}

type Options struct {
	// Repanic configures whether Sentry should repanic after recovery, in cases where your application is utilizing
	// the fiber Recovery middleware it can be set to true, as fiber does not include a recovery middleware by default
	Repanic bool
	// WaitForDelivery configures whether you want to block the request before moving forward with the response.
	// Because Fiber's recovery middleware does restart the application, setting this to true, will block
	// the restart of the application until sentry has sent all remaining requests or until Timeout is reached,
	// whichever comes first.
	WaitForDelivery bool
	// Timeout for the event delivery requests.
	Timeout time.Duration
}

// New returns a function that satisfies gin.HandlerFunc interface
// It can be used with Use() methods.
func New(options ...Options) fiber.Handler {
	opts := Options{
		Repanic:         true,
		WaitForDelivery: true,
		Timeout:         2 * time.Second,
	}
	if len(options) == 1 {
		opts = options[0]
	}

	timeout := opts.Timeout
	if timeout == 0 {
		timeout = 2 * time.Second
	}
	return (&handler{
		repanic:         opts.Repanic,
		timeout:         timeout,
		waitForDelivery: opts.WaitForDelivery,
	}).handle
}

func convert(ctx *fasthttp.RequestCtx) *http.Request {
	defer func() {
		if err := recover(); err != nil {
			sentry.Logger.Printf("%v", err)
		}
	}()

	r := new(http.Request)

	r.Method = string(ctx.Method())
	uri := ctx.URI()
	// Ignore error.
	r.URL, _ = url.Parse(fmt.Sprintf("%s://%s%s", uri.Scheme(), uri.Host(), uri.Path()))

	// Headers
	r.Header = make(http.Header)
	r.Header.Add("Host", string(ctx.Host()))
	ctx.Request.Header.VisitAll(func(key, value []byte) {
		r.Header.Add(string(key), string(value))
	})
	r.Host = string(ctx.Host())

	// Cookies
	ctx.Request.Header.VisitAllCookie(func(key, value []byte) {
		r.AddCookie(&http.Cookie{Name: string(key), Value: string(value)})
	})

	// Env
	r.RemoteAddr = ctx.RemoteAddr().String()

	// QueryString
	r.URL.RawQuery = string(ctx.URI().QueryString())

	// Body
	r.Body = ioutil.NopCloser(bytes.NewReader(ctx.Request.Body()))

	return r
}

func (h *handler) handle(ctx *fiber.Ctx) error {
	hub := sentry.GetHubFromContext(ctx.Context())
	if hub == nil {
		hub = sentry.CurrentHub().Clone()
	}
	hub.Scope().SetRequest(convert(ctx.Context()))
	ctx.Locals(valuesKey, hub)
	defer h.recoverWithSentry(hub, convert(ctx.Context()))
	return ctx.Next()
}

func (h *handler) recoverWithSentry(hub *sentry.Hub, r *http.Request) {
	if err := recover(); err != nil {
		if !isBrokenPipeError(err) {
			eventID := hub.RecoverWithContext(
				context.WithValue(r.Context(), sentry.RequestContextKey, r),
				err,
			)
			if eventID != nil && h.waitForDelivery {
				hub.Flush(h.timeout)
			}
		}
		if h.repanic {
			panic(err)
		}
	}
}

// Check for a broken connection, as this is what Fiber does already.
func isBrokenPipeError(err interface{}) bool {
	if netErr, ok := err.(*net.OpError); ok {
		if sysErr, ok := netErr.Err.(*os.SyscallError); ok {
			if strings.Contains(strings.ToLower(sysErr.Error()), "broken pipe") ||
				strings.Contains(strings.ToLower(sysErr.Error()), "connection reset by peer") {
				return true
			}
		}
	}
	return false
}

// GetHubFromContext retrieves attached *sentry.Hub instance from fiber.Ctx.
func GetHubFromContext(ctx *fiber.Ctx) *sentry.Hub {
	hub := ctx.Locals(valuesKey)
	if hub != nil {
		return hub.(*sentry.Hub)
	}

	return nil
}
