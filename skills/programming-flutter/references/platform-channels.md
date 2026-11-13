# Flutter Platform Channels

Read this when you add or change a platform channel or a plugin that wraps native code.

## Platform Channels — Principles

- Wrap every channel in a typed Dart class with a `Future`-returning API. Never expose raw `MethodChannel` to the rest of the app.
- **Version your method names** (`"v1/getBattery"`) — platform code ships on a different cadence from Dart.
- **Handle `MissingPluginException`** — happens on hot restart and on platforms that don't implement the channel.
- Prefer **Pigeon** for anything non-trivial: generates typed host/Dart code from one IDL, eliminates stringly-typed bugs.
