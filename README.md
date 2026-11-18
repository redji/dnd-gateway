# dnd-gateway

A small Windows backend that fronts two public APIs:

- [D&D 5e SRD API](https://www.dnd5eapi.co/) for rules data (monsters, spells, classes, and the rest of the catalog)
- [Dice Roll API](https://randomapi.dev/apis/dice) for RPG notation such as `2d6+3` and `d20`

One executable listens for JSON over HTTP. Dice rolls go through an internal gRPC service, which is what talks to the dice API. A combined action endpoint loads a rules entry and rolls dice at the same time.

## Run

Go 1.22 or newer is required. From this folder in PowerShell:

```powershell
go mod tidy
go test ./...
go build -o bin\dnd-gateway.exe .\cmd\gateway
.\bin\dnd-gateway.exe
```

The HTTP API listens on `http://127.0.0.1:8080`. The dice gRPC service listens on `127.0.0.1:50051`. Both stay on localhost unless you change the addresses.

```powershell
Invoke-RestMethod http://127.0.0.1:8080/health
Invoke-RestMethod http://127.0.0.1:8080/v1/dnd/monsters/goblin
Invoke-RestMethod "http://127.0.0.1:8080/v1/dice/roll?notation=2d6%2B3&count=2"
Invoke-RestMethod http://127.0.0.1:8080/v1/actions -Method Post -ContentType "application/json" -Body '{"resource":"monsters","index":"goblin","notation":"1d20+2"}'
```

`notation=2d6+3` is also accepted. A raw `+` in a query string is a space, and this gateway treats that space as the modifier plus, matching the dice API.

## Endpoints

| Method | Path | What it does |
| --- | --- | --- |
| GET | `/health` | Process is up |
| GET | `/v1` | Short route list |
| GET | `/v1/dnd` | Catalog of D&D resources |
| GET | `/v1/dnd/{resource}` | List a resource. The query string is forwarded (`name`, and any other filter the upstream supports) |
| GET | `/v1/dnd/{resource}/{index}` | One record, for example `/v1/dnd/spells/fire-ball` |
| GET | `/v1/dice/roll` | Roll dice. Query: `notation`, optional `count` (1–100), optional `seed` |
| POST | `/v1/actions` | JSON body `resource`, `index`, `notation`, optional `count` and `seed`. Returns the rules record and the rolls |

D&D responses are the upstream JSON. Gateway errors look like `{"error":"..."}`.

Resource names are lowercase slugs (`adult-red-dragon`). The gateway does not accept paths that could leave the two configured upstream hosts.

## Process modes

`-mode all` (the default) runs HTTP and gRPC in one process.

To split them, which is the same layout you would deploy as two services:

```powershell
.\bin\dnd-gateway.exe -mode grpc -grpc 127.0.0.1:50051
.\bin\dnd-gateway.exe -mode http -grpc 127.0.0.1:50051 -http 127.0.0.1:8080
```

## Configuration

Flags and environment variables set the same fields. A flag on the command line wins over the environment.

| Flag | Environment | Default |
| --- | --- | --- |
| `-mode` | `GATEWAY_MODE` | `all` |
| `-http` | `GATEWAY_HTTP_ADDR` | `127.0.0.1:8080` |
| `-grpc` | `DICE_GRPC_ADDR` | `127.0.0.1:50051` |
| `-dnd` | `DND_BASE_URL` | `https://www.dnd5eapi.co` |
| `-dnd-version` | `DND_API_VERSION` | `2014` (`2024` is also accepted) |
| `-dice` | `DICE_BASE_URL` | `https://randomapi.dev` |

Ctrl+C stops the process and drains in-flight HTTP requests.

## Layout

`cmd/gateway` is the Windows process. `internal/httpapi` is the REST gateway. `internal/dnd` calls the D&D API. `internal/diceserver` is the gRPC service, and `internal/diceupstream` calls the public dice API. `proto/dice/v1/dice.proto` is the internal contract; `internal/dicepb` is the matching Go API (JSON over gRPC so you do not need `protoc` to build).
