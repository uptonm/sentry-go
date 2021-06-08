<p align="center">
  <a href="https://sentry.io" target="_blank" align="center">
    <img src="https://sentry-brand.storage.googleapis.com/sentry-logo-black.png" width="280">
  </a>
  <br />
</p>

# Official Sentry Gin Handler for Sentry-go SDK

**Godoc:** https://godoc.org/github.com/getsentry/sentry-go/fiber

**Example:** https://github.com/getsentry/sentry-go/tree/master/example/fiber

## Installation

```sh
go get github.com/getsentry/sentry-go/fiber
```

```go
import (
    "fmt"
    "net/http"

    "github.com/getsentry/sentry-go"
    sentryfiber "github.com/getsentry/sentry-go/fiber"
    "github.com/gofiber/fiber/v2"
)

// To initialize Sentry's handler, you need to initialize Sentry itself beforehand
if err := sentry.Init(sentry.ClientOptions{
    Dsn: "your-public-dsn",
}); err != nil {
    fmt.Printf("Sentry initialization failed: %v\n", err)
}

// Then create your app
router := fiber.New()

// Once it's done, you can attach the handler as one of your middleware
router.Use(sentryfiber.New(sentryfiber.Options{}))

// Set up routes
router.Get("/", func(ctx fiber.Ctx) {
    ctx.SendString("Hello world!")
})

// And run it
router.Listen(":3000")
```

## Configuration

`sentryfiber` accepts a struct of `Options` that allows you to configure how the handler will behave.

Currently it respects 3 options:

```go
// Whether or not sentry should repanic after recovery. It should only be set to true if
// your application is utilizing the fiber recovery middleware
Repanic         bool
// Whether you want to block the request before moving forward with the response.
// Because Fiber's default `Recovery` handler doesn't restart the application,
// it's safe to either skip this option or set it to `false`.
WaitForDelivery bool
// Timeout for the event delivery requests.
Timeout         time.Duration
```

## Usage

`sentryfiber` attaches an instance of `*sentry.Hub` (https://godoc.org/github.com/getsentry/sentry-go#Hub) to the `*fiber.Ctx`, which makes it available throughout the rest of the request's lifetime.
You can access it by using the `sentryfiber.GetHubFromContext()` method on the context itself in any of your proceeding middleware and routes.
And it should be used instead of the global `sentry.CaptureMessage`, `sentry.CaptureException`, or any other calls, as it keeps the separation of data between the requests.

**Keep in mind that `*sentry.Hub` won't be available in middleware attached before to `sentryfiber`!**

```go
router := fiber.New()

router.Use(sentryfiber.New(sentryfiber.Options{
    Repanic: true,
}))

router.Use(func(ctx *fiber.Ctx) {
    if hub := sentryfiber.GetHubFromContext(ctx); hub != nil {
        hub.Scope().SetTag("someRandomTag", "maybeYouNeedIt")
    }
    ctx.Next()
})

router.Get("/", func(ctx *fiber.Ctx) {
    if hub := sentryfiber.GetHubFromContext(ctx); hub != nil {
        hub.WithScope(func(scope *sentry.Scope) {
            scope.SetExtra("unwantedQuery", "someQueryDataMaybe")
            hub.CaptureMessage("User provided unwanted query string, but we recovered just fine")
        })
    }
    ctx.Status(http.StatusOK)
})

router.Get("/foo", func(ctx *fiber.Ctx) {
    // sentryfiber handler will catch it just fine. Also, because we attached "someRandomTag"
    // in the middleware before, it will be sent through as well
    panic("y tho")
})

router.Listen(":3000")
```

### Accessing Request in `BeforeSend` callback

```go
sentry.Init(sentry.ClientOptions{
    Dsn: "your-public-dsn",
    BeforeSend: func(event *sentry.Event, hint *sentry.EventHint) *sentry.Event {
        if hint.Context != nil {
            if req, ok := hint.Context.Value(sentry.RequestContextKey).(*http.Request); ok {
                // You have access to the original Request here
            }
        }

        return event
    },
})
```
