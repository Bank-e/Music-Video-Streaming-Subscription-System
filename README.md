# Music-Video-Streaming-Subscription-System

## Project Structure

```text
Streaming-Subscription-System/
│
├── identity/                # Identity Service
│   ├── handler/             # User Controller (HTTP requests & responses)
│   ├── service/             # User business logic
│   ├── repository/          # User, Profile, UserPreference database operations
│   └── domain/              # User, Profile, UserPreference structs
│
├── auth/                    # Authentication Service
│   ├── handler/             # Auth Controller
│   ├── service/             # Auth business logic
│   ├── repository/          # Session database operations
│   └── domain/              # UserSession, AccessPrincipal structs
│
├── billing/                 # Billing & Subscription Service
│   ├── handler/             # Billing Controller
│   ├── service/             # Billing business logic
│   ├── repository/          # Plan, Subscription, Payment database operations
│   └── domain/              # Plan, Subscription, Payment, Enums
│
└── content/                 # Content & History Service
    ├── handler/             # Content & WatchHistory Controllers
    ├── service/             # Content & WatchHistory business logic
    ├── repository/          # Video, Music, WatchHistory database operations
    └── domain/              # Content, Video, Music, WatchHistory structs
```
