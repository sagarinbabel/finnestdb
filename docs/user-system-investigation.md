# User System Investigation

## Current state on `main`

The repo has partial user-related tables and stub handlers, but not a real user system.

### What exists

- `users` table with:
  - `id`
  - `email`
  - `email_verified`
  - `settings_json`
- deck/card ownership already keyed by `user_id`
- `POST /api/auth/login`
- `GET /api/me`

### What is stubbed

- login accepts any email and creates the user automatically
- the auth cookie stores raw `user_id`
- requests without a cookie silently fall back to user `1`
- there is no password validation
- there is no session store
- there is no logout
- there is no registration flow separate from login
- there is no middleware that distinguishes authenticated vs unauthenticated requests

Relevant code:

- [internal/api/handlers.go](/Users/basati/Documents/Programming/web/finnestdb/internal/api/handlers.go)
- [internal/store/db.go](/Users/basati/Documents/Programming/web/finnestdb/internal/store/db.go)

## Main problems

### 1. Identity is client-spoofable

The current cookie stores the numeric user ID directly. That is not a real session.

### 2. Guest access is implemented as shared user `1`

This is the wrong model once the app has private data.

The issue is not that guest access exists. The issue is that all unauthenticated
requests collapse into one implicit shared account.

That causes:

- mixed guest state across visitors
- accidental writes to a shared account
- broken ownership expectations for private decks and study data
- ambiguous access-control logic because "not logged in" looks like a real user

### 3. No separation between auth and user profile

The current `users` table is doing double duty as both login identity and all
user-facing account data.

This is not inherently wrong, but if we want a clear split between auth data
and product/profile data, the schema should make that distinction explicitly.

### 4. No server-side session invalidation

There is no way to revoke a session, expire one centrally, or inspect active sessions.

### 5. No access-control foundation

Private deck ownership checks in the SRS/deck spec require a trustworthy authenticated user context first.

## Recommendation

Use:

- email + password for MVP
- server-side opaque sessions in SQLite
- secure HTTP-only cookie carrying a random session token

Do not use:

- client-supplied user IDs
- raw `user_id` cookies
- default fallback users outside explicit dev-only code paths

## User types

The system should distinguish between access mode and authenticated account type.

### Guest

- not an authenticated user account
- can browse public/shared media
- cannot use SRS features
- cannot have personal study state
- should not map to a shared fallback `users` row

### Free user

- authenticated user
- can use the study system
- currently has the same study capabilities as paid users

### Paid user

- authenticated user
- currently same as free for study features
- should exist as a separate tier now so feature gating can be added later

### Admin user

- authenticated user with elevated permissions
- can upload and manage public/shared media
- can edit admin-only account/media data

### Recommendation

Represent this as:

- unauthenticated request => guest access mode
- authenticated request => user account
- user account carries admin/tier metadata

Do not treat guest as a normal user row by default.

## Why this is the best MVP

Compared with magic links:

- no email delivery dependency
- easier local development
- fewer moving parts for the first real auth system

Compared with Google OAuth first:

- no external provider setup required
- easier to implement and test locally
- can still add OAuth later as an additional login method

This is not the final auth surface forever. It is the cleanest self-contained first step for this codebase.

## Recommended model

### Core tables

- `users`
  - authentication/account record

- `user_profiles`
  - required one-to-one profile record for each user

- `user_sessions`
  - `id`
  - `user_id`
  - `token_hash`
  - `created_at`
  - `expires_at`
  - `revoked_at` nullable
  - `last_seen_at` nullable
  - `ip_address` nullable
  - `user_agent` nullable

- `password_reset_tokens`
  - `id`
  - `user_id`
  - `token_hash`
  - `created_at`
  - `expires_at`
  - `used_at` nullable

### Extend `users`

Keep:

- `id`
- `email`
- `email_verified`
- `password_hash`

Add:

- `created_at`
- `updated_at`
- `is_admin`
- `plan`

Recommended values:

- `plan`: `free`, `paid`

Guest should not be stored here unless we intentionally add anonymous accounts later.

### `user_profiles`

Every user should have a profile row created at signup.

Recommended fields:

- `user_id`
- `settings_json`
- `display_name` nullable
- `created_at`
- `updated_at`

Optional later:

- `status`
- avatar fields
- profile metadata

## Session design

### Cookie format

Use one opaque session cookie, for example:

- `session_token`

The cookie value should be a random secret, not a user ID.

### Storage

Store only a hash of the session token in the database.

That way, a DB leak does not expose live session secrets directly.

### Cookie settings

- `HttpOnly`
- `Secure` in production
- `SameSite=Lax` or `Strict`
- explicit `Path=/`
- explicit expiration

## Password design

Store `password_hash` on `users`.

Use Argon2id for password hashing.

Do not store:

- plaintext passwords
- reversible encrypted passwords
- unsalted fast hashes

For password reset tokens:

- store only a hash of the reset token
- make tokens single-use
- give tokens a short expiration window
- return a generic success response from reset-request endpoints even if the email does not exist

## Recommended API shape

### Auth

- `POST /api/auth/register`
  - input: `{email, password}`
  - creates user, creates user profile, creates session

- `POST /api/auth/login`
  - input: `{email, password}`
  - verifies password, creates session

- `POST /api/auth/logout`
  - revokes current session

- `GET /api/me`
  - returns authenticated user info

- `POST /api/auth/request-password-reset`
  - input: `{email}`
  - creates a reset token and sends a reset email

- `POST /api/auth/reset-password`
  - input: `{token, new_password}`
  - verifies token, updates password, invalidates token, revokes existing sessions

### Optional later

- `POST /api/auth/logout-all`
- `POST /api/auth/change-password`

## Middleware requirements

Add a real auth middleware/helper that:

1. reads `session_token` from cookie
2. hashes it
3. loads the session
4. verifies not expired and not revoked
5. loads the user
6. places authenticated user info into request context

Then split handlers into:

- public handlers
- guest-only or guest-safe handlers
- authenticated handlers
- authenticated + owner-checked handlers

Recommended capability split:

- guest
  - public media browse/read only
  - no coverage, no study state, no private data

- free
  - authenticated study features

- paid
  - same as free for now
  - future paid-only features can branch from `plan`

- admin
  - admin management features on top of authenticated access
  - should be treated as having paid-level product access regardless of `plan`

## Deck ownership implications

This user system needs to support the deck rules already documented in the SRS/deck spec:

- shared catalog decks are public/shared
- private decks are owned by one user
- private deck reads must be gated by authenticated ownership checks

That means deck handlers should never trust request payload for user identity.

They should always use the authenticated user from request context.

## Migration from current code

### Phase 1

- remove fallback to user `1`
- replace raw `user_id` cookie with real `session_token`
- add `user_sessions`
- add `password_hash` to `users`
- add `user_profiles`
- add `password_reset_tokens`
- make `/api/me` require authentication

### Phase 2

- add `register`
- make `login` validate password instead of auto-creating users
- add `logout`
- add password reset request/reset endpoints

### Phase 3

- update deck/study handlers to require authenticated context where appropriate
- enforce owner checks for private deck data

### Phase 4

- add email verification or OAuth if needed

## Open decisions

### 1. Email verification now or later

Recommendation:

- later

Reason:

- it requires outbound email infrastructure
- it is not necessary to establish trustworthy sessions

### 2. OAuth now or later

Recommendation:

- later

Reason:

- it adds provider setup and callback handling
- it is easier to layer on top of a working local account/session system

### 3. Transactional email for password reset

Recommendation:

- required for MVP if password auth is in MVP

Reason:

- password reset is part of the MVP auth flow
- that means we will need email delivery for reset links

## Concrete next step

The next useful step is not frontend design yet.

It is to define and implement the backend auth foundation:

1. schema changes for `users`, `user_profiles`, and `user_sessions`
2. add `password_reset_tokens`
3. auth middleware
4. register/login/logout/me/password-reset endpoints
5. removal of the default-user stub

Once that exists, the deck/study privacy model can be implemented safely.
