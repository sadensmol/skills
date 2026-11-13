# Swift Codable and Persistence

Read this when you add or change a persisted `Codable` type.

## Codable & Persistence

The rules are `sadensmol-programming` → *Backward-Compatible Data*. In `Codable`:

- Emit a new optional field only when non-default: synthesized `encodeIfPresent`, or a custom `encode(to:)`.
- Decode leniently in a custom `init(from:)`: `(try? c.decodeIfPresent(T.self, forKey: .x)) ?? default`.
- Store the inverse as `collapsed: Bool?`, encoded only when `true`.
