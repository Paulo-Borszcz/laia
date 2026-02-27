# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

**Laia** is a WhatsApp bot (Go) that bridges WhatsApp Cloud API (Meta) with a GLPI instance called **Nexus**. It receives messages via webhook, authenticates users against GLPI's REST API, and provides GLPI service desk functionality through WhatsApp conversations.

## Architecture

```
WhatsApp User ──► Meta Cloud API ──► Azure (webhook) ──► Laia (Go)
                                                            │
                                                            ├── WhatsApp Cloud API (send replies)
                                                            ├── Nexus GLPI REST API (tickets, assets, etc.)
                                                            └── Auth service (user_token verification page)
```

### Authentication Flow
1. Unknown user sends message → bot replies with a verification URL
2. User opens the URL, enters their GLPI `user_token` (found in GLPI User Preferences → Remote access key)
3. Backend calls `GET /apirest.php/initSession` with `Authorization: user_token <token>` + `App-Token` header to validate
4. On success, the WhatsApp number is linked to the GLPI user (persisted)
5. Subsequent messages are handled as the authenticated GLPI user

### GLPI (Nexus) API
- Full REST API docs: `nexus_apirest.md`
- Base URL: env `NEXUS_BASE_URL` (https://nexus.lojasmm.com.br)
- Every request requires `App-Token` header (env `NEXUS_APP_TOKEN`)
- Per-user requests require `Session-Token` obtained via `initSession` with `user_token`
- Sessions are read-only by default; pass `session_write=true` for write operations
- Always send `Content-Type: application/json`; GET requests must have empty body

### WhatsApp Cloud API
- Docs: https://developers.facebook.com/docs/whatsapp/cloud-api
- Phone Number ID: env `WA_PHONE_NUMBER_ID`
- Access Token: env `WA_ACCESS_TOKEN`
- Webhook verify token: env `WA_VERIFY_TOKEN` (generated secret for webhook registration)
- Webhook must respond to GET (verification challenge) and POST (incoming messages)

## Environment Variables (.env)

```
NEXUS_BASE_URL=https://nexus.lojasmm.com.br
NEXUS_APP_TOKEN=<app token from GLPI>
WA_PHONE_NUMBER_ID=<phone number ID from Meta>
WA_ACCESS_TOKEN=<access token from Meta>
WA_VERIFY_TOKEN=<generated secret for webhook>
PORT=8080
```

Secrets are configured as Azure App Service settings and GitHub Actions secrets.

## Build & Run Commands

```bash
go build -o laia ./cmd/laia          # build binary
go run ./cmd/laia                     # run locally
go test ./...                         # run all tests
go test ./internal/glpi -run TestX    # run a single test
go vet ./...                          # lint
```

## Deployment (Azure)

Infrastructure is provisioned via Azure CLI (`az`). Key resources:
- Azure App Service (or Container App) for the webhook + auth page
- The webhook URL and verify token must be registered in Meta's WhatsApp Business dashboard

## Key Design Decisions

- All secrets come from environment variables (loaded from `.env` in dev)
- GLPI sessions are short-lived: `initSession` → do work → `killSession`
- WhatsApp number ↔ GLPI user mapping is the core persistence requirement
- The auth verification page is a simple HTML form served by the same Go server

## Comentarios no Codigo

Seguir estas 9 regras ao escrever comentarios (baseado em boas praticas):

1. **Nao duplicar o codigo** - Nunca escrever comentarios que apenas repetem o que o codigo ja diz. Se o nome da funcao/variavel ja explica, nao comentar.
2. **Comentarios nao desculpam codigo confuso** - Se o codigo precisa de um comentario longo para ser entendido, refatorar o codigo primeiro.
3. **Se nao consegue escrever um comentario claro, o codigo tem problema** - Dificuldade em comentar indica que o codigo precisa ser reescrito.
4. **Comentarios devem eliminar confusao, nao criar** - Sem comentarios cripticos ou ambiguos.
5. **Explicar codigo nao-idiomatico** - Documentar padroes incomuns (ex: `unsafe`, workarounds, `danger_accept_invalid_certs`).
6. **Incluir links para codigo copiado** - Quando copiar solucoes de StackOverflow/docs, incluir a URL de origem.
7. **Links para referencias externas** - Referenciar RFCs, docs de APIs externas, endpoints consultados.
8. **Comentar ao corrigir bugs** - Referenciar issues (`#123`), explicar workarounds e por que a solucao nao-obvia foi escolhida.
9. **Marcar implementacoes incompletas** - Usar `// TODO:` com contexto suficiente para entender o que falta.
