# Ports and Adapters

The two outer layers. **Ports** are entry points: HTTP handlers, gRPC handlers, message subscribers, CLI commands. **Adapters** are exits: database clients, external service clients, message publishers.

Together they form the boundary between your service and everything else. The application and domain layers don't know they exist.

> Terminology note: in classic Hexagonal Architecture, both inputs and outputs are called "adapters" (primary and secondary). This skill follows *Go with the Domain*'s naming — entry points are `ports/`, exits are `adapters/` — because the words are easier to keep straight.

## Ports

A port handler:

1. Decodes the transport-specific request (JSON, protobuf).
2. Extracts cross-cutting context (auth user, request ID).
3. Builds the relevant command or query struct, using **domain types** for parameters.
4. Calls `h.app.Commands.Whatever.Handle(ctx, cmd)` or the query equivalent.
5. Translates the result back to transport (status code, response body).

That's it. No business logic. No SQL. No external service calls.

```go
// in ports/http.go

package ports

import (
    "net/http"

    "github.com/go-chi/chi/v5"

    "yourproject/internal/trainings/app"
    "yourproject/internal/trainings/app/command"
    "yourproject/internal/trainings/domain/training"
    "yourproject/internal/common/server/httperr"
)

type HttpServer struct {
    app app.Application
}

func NewHttpServer(application app.Application) HttpServer {
    return HttpServer{app: application}
}

func (h HttpServer) CancelTraining(w http.ResponseWriter, r *http.Request) {
    trainingUUID := chi.URLParam(r, "trainingUUID")

    user, err := newDomainUserFromAuthUser(r.Context())
    if err != nil {
        httperr.RespondWithSlugError(err, w, r)
        return
    }

    err = h.app.Commands.CancelTraining.Handle(r.Context(), command.CancelTraining{
        TrainingUUID: trainingUUID,
        User:         user,
    })
    if err != nil {
        httperr.RespondWithSlugError(err, w, r)
        return
    }

    w.WriteHeader(http.StatusNoContent)
}
```

The `newDomainUserFromAuthUser` helper converts the transport-level auth representation (whatever the JWT middleware produced) into the domain `User` value object. This is the only port-specific translation needed.

## Generated code in ports

If you use OpenAPI or protobuf, the generated code goes in `ports/`:

```text
ports/
├── http.go               # your handlers
├── grpc.go
├── openapi_api.gen.go    # generated server interface
├── openapi_types.gen.go  # generated request/response types
└── grpc_server.gen.go    # generated gRPC service interface
```

Implement the generated interface with your `HttpServer` / `GrpcServer`. Use `oapi-codegen` for OpenAPI and `protoc-gen-go-grpc` for gRPC. The generated types are NOT used outside `ports/`.

## Returning created resources via POST (CQRS-style)

A natural friction: REST suggests `POST /trainings → returns the created training`. But CQRS says commands don't return data. Two clean options:

**Option 1: Return `204 No Content` with `Content-Location`.** The client follows the link if it wants the full resource:

```go
cmd := command.ScheduleTraining{
    TrainingUUID: uuid.New().String(),  // generated client-side
    UserUUID:     user.UUID,
    Time:         req.Time,
    Notes:        req.Notes,
}
if err := h.app.Commands.ScheduleTraining.Handle(r.Context(), cmd); err != nil {
    httperr.RespondWithSlugError(err, w, r)
    return
}
w.Header().Set("Content-Location", "/trainings/"+cmd.TrainingUUID)
w.WriteHeader(http.StatusNoContent)
```

Note the UUID is generated *before* the command runs. The client can use it immediately without waiting for a response body. This also works with async commands.

**Option 2: Run the command, then call a query to fetch the result.** More HTTP-like but defeats some caching advantages:

```go
if err := h.app.Commands.ScheduleTraining.Handle(ctx, cmd); err != nil { ... }
training, err := h.app.Queries.TrainingDetails.Handle(ctx, query.TrainingDetails{UUID: cmd.TrainingUUID})
if err != nil { ... }
json.NewEncoder(w).Encode(training)
```

Pick one and apply it consistently.

## Adapters

An adapter implements an interface declared by an inner layer:

- A `Repository` implementation (declared in `domain/<entity>/repository.go`) → lives in `adapters/`.
- A `UserService` mock-friendly interface (declared in `app/command/services.go`) → its real implementation in `adapters/`.

Adapter types translate between domain types and the external world's types. They convert errors to or from domain errors. They handle retries, timeouts, and circuit breakers (often via middleware on the underlying client).

```go
// in adapters/trainer_grpc.go

package adapters

import (
    "context"
    "time"

    "google.golang.org/protobuf/types/known/timestamppb"

    "yourproject/internal/common/genproto/trainer"
)

type TrainerGrpc struct {
    client trainer.TrainerServiceClient
}

func NewTrainerGrpc(client trainer.TrainerServiceClient) TrainerGrpc {
    return TrainerGrpc{client: client}
}

func (s TrainerGrpc) CancelTraining(ctx context.Context, trainingTime time.Time) error {
    _, err := s.client.CancelTraining(ctx, &trainer.UpdateHourRequest{
        Time: timestamppb.New(trainingTime),
    })
    return err
}

func (s TrainerGrpc) MoveTraining(ctx context.Context, newTime, oldTime time.Time) error {
    if err := s.CancelTraining(ctx, oldTime); err != nil {
        return err
    }
    return s.scheduleTraining(ctx, newTime)
}
```

The interface this satisfies (`TrainerService`) lives in `app/command/`, not here. This adapter has no idea what the application layer expects — it just implements the methods. Compile time tells you whether it matches.

## main.go — wiring it all together

For small services, plain function calls in `main.go` are perfectly clear:

```go
// in main.go

package main

import (
    "context"
    "log"
    "os"

    "yourproject/internal/trainings/adapters"
    "yourproject/internal/trainings/app"
    "yourproject/internal/trainings/app/command"
    "yourproject/internal/trainings/app/query"
    "yourproject/internal/trainings/ports"
    "yourproject/internal/common/server"
)

func main() {
    ctx := context.Background()

    // adapters
    firestoreClient, cleanup := newFirestoreClient(ctx)
    defer cleanup()

    trainingsRepo := adapters.NewTrainingsFirestoreRepository(firestoreClient)
    trainerGrpc := adapters.NewTrainerGrpc(newTrainerGrpcClient())
    usersGrpc := adapters.NewUsersGrpc(newUsersGrpcClient())

    // application
    application := app.Application{
        Commands: app.Commands{
            ScheduleTraining: command.NewScheduleTrainingHandler(trainingsRepo, usersGrpc, trainerGrpc),
            CancelTraining:   command.NewCancelTrainingHandler(trainingsRepo, usersGrpc, trainerGrpc),
            // ...
        },
        Queries: app.Queries{
            AllTrainings:    query.NewAllTrainingsHandler(trainingsRepo),
            TrainingDetails: query.NewTrainingDetailsHandler(trainingsRepo),
            // ...
        },
    }

    // ports
    httpServer := ports.NewHttpServer(application)

    log.Fatal(server.RunHTTPServer(os.Getenv("HTTP_ADDR"), httpServer))
}
```

You can see at a glance how every dependency flows. For services with 20+ handlers, [`google/wire`](https://github.com/google/wire) generates this wiring from declarations.

## Constructors should fail fast

Every constructor for a port, adapter, application, or handler should `panic` on `nil` dependencies. This catches misconfigurations at startup, not under load:

```go
func NewCancelTrainingHandler(
    repo training.Repository,
    userService UserService,
    trainerService TrainerService,
) CancelTrainingHandler {
    if repo == nil {
        panic("nil repo")
    }
    if userService == nil {
        panic("nil userService")
    }
    if trainerService == nil {
        panic("nil trainerService")
    }
    return CancelTrainingHandler{repo, userService, trainerService}
}
```

`panic` in `main()` startup is fine — your container will refuse to come up, and your deployment system will roll back.

## Component-test mode

Tests at the component level run the real ports against mocked external services. Provide a second factory for this:

```go
// in service/service.go (or wherever you build the Application)

func NewApplication(ctx context.Context) (app.Application, func() /* cleanup */) {
    // real adapters: real DB, real gRPC clients
    return newApplication(ctx, realTrainerService, realUserService), cleanup
}

func NewComponentTestApplication(ctx context.Context) app.Application {
    // real DB (or in-memory), mock external services
    return newApplication(ctx, TrainerServiceMock{}, UserServiceMock{})
}

func newApplication(
    ctx context.Context,
    trainerService command.TrainerService,
    userService command.UserService,
) app.Application {
    // ... build everything else
}
```

The `TrainerServiceMock` and `UserServiceMock` live in a test-only package or under `// +build component` files. Component tests import them; production code doesn't.

See `references/testing.md` for how component tests use this.

## Enforcing the layer rules

You can enforce the import direction in CI with [`go-cleanarch`](https://github.com/roblaszczak/go-cleanarch):

```bash
go-cleanarch -domain domain -application app -infrastructure adapters -interfaces ports
```

This catches any commit that, say, imports `adapters` from inside `domain` — the kind of accident that's easy to make under deadline pressure and hard to undo three releases later.

## Common mistakes

- **Logic in port handlers.** Validation of business rules belongs in the domain. The port should only validate transport-level concerns (JSON shape, required fields). If your `http.go` has more than ~30 lines per endpoint, look hard at what should move into `app/` or `domain/`.
- **Returning generated DB or transport types from `app/` or `domain/`.** The OpenAPI struct is a `ports/` type. The `mysqlTraining` struct is an `adapters/` type. The domain `Training` is something else again. Don't blur them.
- **Using `auth.User` (or similar transport-auth type) inside `app/` or `domain/`.** Translate it into a domain `User` value object at the port boundary. The auth library is an implementation detail; the inner layers shouldn't know it exists.
- **Putting all interfaces in one big `interfaces.go`.** Co-locate the interface with its consumer. Three separate small interfaces are better than one fat one.
