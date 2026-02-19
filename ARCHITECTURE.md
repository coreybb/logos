# Logos

Logos is a personal magazine generator. You subscribe to newsletters using your Logos inbox address, and Logos automatically bundles them into ebooks delivered to your Kindle on a schedule you define.

## How It Works

### 1. You get an inbox

When you create an account, Logos gives you an email address:

```
inbox+{your-user-id}@parse.lakonic.dev
```

This is the address you use when subscribing to newsletters, Substacks, blogs — anything that sends email.

### 2. Content arrives automatically

Newsletters land in your Logos inbox. The app:

- Parses the email and extracts the article content
- Strips formatting cruft, pulls the title and author
- Deduplicates by content hash (same article twice = stored once)
- Auto-discovers the sender as a **reading source**

No forwarding, no manual import. Content flows in directly.

### 3. You triage new sources

When a new sender appears for the first time, it shows up as an **unassigned source** — awaiting triage. From the UI, you decide what to do with it:

- **Assign it to a magazine** — its future readings will be included in that magazine's editions
- **Ignore it** — it sits unassigned; its readings never appear in any edition

You can assign a single source to multiple magazines, or reassign it later.

### 4. Magazines generate on schedule

A magazine (internally called an **edition template**) defines:

- **Name** — "Morning Reads", "Weekly Deep Dives", etc.
- **Format** — EPUB (Kindle-compatible)
- **Delivery interval** — hourly, daily, weekly, monthly
- **Delivery time** — what time of day to deliver (e.g., 07:00)

When a magazine's schedule fires, Logos:

1. Looks up which sources are assigned to this magazine
2. Gathers all readings from those sources since the last edition
3. Combines them into a single HTML document
4. Generates an EPUB
5. Emails the EPUB to your delivery destination (e.g., your Kindle email)

If there are no new readings from assigned sources, nothing happens — no empty editions.

### 5. Your Kindle gets a new book

The EPUB arrives as an email attachment to your Kindle address. Amazon processes it and it appears in your library, ready to read.

## Core Concepts

| Concept | What it is |
|---------|-----------|
| **User** | An account with a unique inbox address |
| **Reading** | A single piece of content (an article, a newsletter issue) |
| **Reading Source** | Where content comes from (auto-created from sender emails) |
| **Edition Template** | A magazine definition — name, format, schedule |
| **Edition** | A specific issue of a magazine, containing one or more readings |
| **Delivery Destination** | Where editions are sent (e.g., a Kindle email address) |
| **Delivery** | A record of an edition being sent to a destination |

## Data Flow

```
Newsletter sender
    |
    v
inbox+{userID}@parse.lakonic.dev
    |
    v
SendGrid Inbound Parse webhook
    |
    v
Inbound Email Handler
    |-- extracts user ID from recipient address
    |-- parses MIME, extracts sender and content
    |
    v
Ingestion Orchestrator
    |-- identifies primary content (HTML body or attachment)
    |-- extracts and sanitizes article content
    |-- auto-creates reading source for sender if new
    |-- deduplicates by content hash
    |-- stores reading in database (content body + metadata)
    |-- links reading to user
    |
    v
Reading stored, source discovered
    |
    v
[User assigns source to magazine via UI/API]
    |
    v
Scheduler tick (triggered by Cloud Scheduler, hourly)
    |
    v
For each recurring edition template:
    |-- check if schedule is due
    |-- fetch source IDs assigned to this template
    |-- fetch readings from those sources since last edition
    |-- skip if no new readings
    |-- create edition, add readings
    |-- generate EPUB from combined HTML
    |-- send via SendGrid to user's delivery destination
    |
    v
EPUB arrives on Kindle
```

## API Endpoints

### Users
- `GET /api/users` — list users
- `POST /api/users` — create user
- `GET /api/users/{id}` — get user
- `GET /api/users/{id}/readings` — get user's readings

### Sources
- `GET /api/sources` — list all sources
- `POST /api/sources` — create source manually
- `GET /api/sources/{id}` — get source
- `GET /api/users/{userID}/sources/unassigned` — sources awaiting triage

### Magazines (Edition Templates)
- `POST /api/edition-templates` — create magazine
- `GET /api/users/{userID}/edition-templates` — list user's magazines
- `GET /api/users/{userID}/edition-templates/{id}` — get magazine
- `PUT /api/users/{userID}/edition-templates/{id}` — update magazine
- `DELETE /api/users/{userID}/edition-templates/{id}` — delete magazine

### Source-to-Magazine Assignment
- `GET /api/edition-templates/{templateID}/sources` — list sources assigned to a magazine
- `POST /api/edition-templates/{templateID}/sources/{sourceID}` — assign source to magazine
- `DELETE /api/edition-templates/{templateID}/sources/{sourceID}` — remove source from magazine

### Delivery Destinations
- `GET /api/destinations?user_id=...` — list destinations
- `POST /api/destinations` — create destination (e.g., Kindle email)

### Allowed Senders
- `GET /api/users/{userID}/allowed-senders` — list allowed senders
- `POST /api/users/{userID}/allowed-senders` — add allowed sender
- `DELETE /api/users/{userID}/allowed-senders/{id}` — remove allowed sender

### Scheduler
- `POST /scheduler/tick` — trigger a scheduler cycle (called by Cloud Scheduler)

### Webhooks
- `POST /webhooks/inbound-email` — SendGrid inbound parse webhook

## Infrastructure

- **Runtime:** Go binary on Google Cloud Run
- **Database:** PostgreSQL (NeonDB)
- **Email inbound:** SendGrid Inbound Parse
- **Email outbound:** SendGrid API v3
- **Scheduling:** Google Cloud Scheduler (hourly HTTP POST to `/scheduler/tick`)
- **Ebook generation:** go-epub (pure Go, no external dependencies)
- **Secrets:** Google Secret Manager
