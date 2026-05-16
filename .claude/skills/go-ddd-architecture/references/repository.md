# The Repository Pattern

The repository abstracts persistence. The domain declares an interface; an outer-layer struct implements it. This is what lets the domain stay database-agnostic, and it's what lets you write unit tests against an in-memory implementation while production uses MySQL or Firestore.

## The interface lives with the domain

Define the interface in the same package as the entity it persists:

```go
// in domain/training/repository.go

package training

type Repository interface {
    AddTraining(ctx context.Context, tr *Training) error

    GetTraining(ctx context.Context, trainingUUID string, user User) (*Training, error)

    UpdateTraining(
        ctx context.Context,
        trainingUUID string,
        user User,
        updateFn func(ctx context.Context, tr *Training) (*Training, error),
    ) error
}
```

Following Go's `io.Writer` pattern: the consumer of the interface defines it. Implementations live in `adapters/`.

## The `updateFn` closure pattern

The single most important pattern in this skill. Instead of separate `Get` / `Save` calls (which open the door to lost-update bugs), the repository takes a callback:

```go
err := repo.UpdateTraining(ctx, trainingUUID, user,
    func(ctx context.Context, tr *training.Training) (*training.Training, error) {
        // 1. Domain logic
        if err := tr.Cancel(); err != nil {
            return nil, err
        }
        // 2. Side effects against external systems, INSIDE the transaction window
        if err := trainerService.CancelTraining(ctx, tr.Time()); err != nil {
            return nil, err
        }
        // 3. Return the entity to persist
        return tr, nil
    },
)
```

Inside the implementation, this becomes:

1. Open a transaction.
2. Fetch the entity, locking it for update (`SELECT ... FOR UPDATE` in SQL, or the equivalent in your DB).
3. Unmarshal into a `*training.Training`.
4. Authorize (see "Secure by design" below).
5. Call `updateFn`. If it returns an error, roll back.
6. Marshal the result back and persist.
7. Commit.

The closure is the transaction boundary. The application layer never sees `*sql.Tx` or `*firestore.Transaction`. Domain code never sees them either.

Earlier attempts in the book to thread transactions through `context.Context` or HTTP middleware were all abandoned for being implicit and slow. The closure is explicit and fast.

If you need more than one return value (e.g. emit events, return a read-model snapshot), add another method or extend the closure signature:

```go
UpdateTrainingWithEvents(
    ctx context.Context,
    uuid string,
    user User,
    updateFn func(ctx context.Context, tr *Training) (*Training, []Event, error),
) error
```

## Separate DB transport types from domain types

The domain `Training` must NOT carry `` `db:"..."` `` tags. Define a parallel struct in the adapter:

```go
// in adapters/trainings_mysql_repository.go

type mysqlTraining struct {
    UUID            string    `db:"uuid"`
    UserUUID        string    `db:"user_uuid"`
    UserName        string    `db:"user_name"`
    Time            time.Time `db:"time"`
    Notes           string    `db:"notes"`
    ProposedNewTime sql.NullTime `db:"proposed_new_time"`
    MoveProposedBy  sql.NullString `db:"move_proposed_by"`
    Canceled        bool      `db:"canceled"`
}

func (r MySQLTrainingsRepository) marshalTraining(t *training.Training) mysqlTraining {
    // map domain → DB
}

func (r MySQLTrainingsRepository) unmarshalTraining(m mysqlTraining) (*training.Training, error) {
    // map DB → domain, going through the domain factory
    return r.factory.UnmarshalTrainingFromDatabase(
        m.UUID, m.UserUUID, m.UserName, m.Time, m.Notes, m.Canceled,
    )
}
```

Yes, the mapping is boilerplate. That is the point — it lets the DB schema and the domain evolve independently. A new column doesn't force you to touch the domain. A new domain method doesn't force a migration.

For the inverse direction (DB → domain), provide an `Unmarshal*FromDatabase` factory method that bypasses the normal constructor validation (the data is already valid because it came from your own write path) but still produces a properly-encapsulated object.

## Transactions — three concrete implementations

### In-memory (for tests and Domain-First exploration)

```go
type MemoryTrainingsRepository struct {
    trainings map[string]training.Training
    lock      sync.RWMutex
}

func (m *MemoryTrainingsRepository) UpdateTraining(
    ctx context.Context,
    uuid string,
    user training.User,
    updateFn func(ctx context.Context, tr *training.Training) (*training.Training, error),
) error {
    m.lock.Lock()
    defer m.lock.Unlock()

    current, ok := m.trainings[uuid]
    if !ok {
        return training.NotFoundError{TrainingUUID: uuid}
    }
    if err := training.CanUserSeeTraining(user, current); err != nil {
        return err
    }

    updated, err := updateFn(ctx, &current)
    if err != nil {
        return err
    }

    // Crucially: store a copy, not the pointer. If updateFn returns an error,
    // we never reach this line and the map is unchanged.
    m.trainings[uuid] = *updated
    return nil
}
```

The copy-by-value detail matters: storing `*current` would mean mutations inside `updateFn` are visible even on rollback. A test should verify this — see `references/testing.md` on the rollback test.

### MySQL (`sqlx`)

```go
func (r MySQLTrainingsRepository) UpdateTraining(
    ctx context.Context,
    uuid string,
    user training.User,
    updateFn func(ctx context.Context, tr *training.Training) (*training.Training, error),
) (err error) {
    tx, err := r.db.Beginx()
    if err != nil {
        return fmt.Errorf("unable to start transaction: %w", err)
    }
    // Named return + defer: this runs even on early return, and a commit
    // failure can overwrite a nil err.
    defer func() {
        err = r.finishTransaction(err, tx)
    }()

    existing, err := r.getTrainingForUpdate(ctx, tx, uuid) // SELECT ... FOR UPDATE
    if err != nil {
        return err
    }
    if err := training.CanUserSeeTraining(user, *existing); err != nil {
        return err
    }

    updated, err := updateFn(ctx, existing)
    if err != nil {
        return err
    }
    return r.upsertTraining(tx, updated)
}

func (r MySQLTrainingsRepository) finishTransaction(err error, tx *sqlx.Tx) error {
    if err != nil {
        if rollbackErr := tx.Rollback(); rollbackErr != nil {
            return multierr.Combine(err, rollbackErr)
        }
        return err
    }
    if commitErr := tx.Commit(); commitErr != nil {
        return fmt.Errorf("failed to commit tx: %w", commitErr)
    }
    return nil
}
```

The `SELECT ... FOR UPDATE` is non-optional for concurrent writes. Without it, two parallel transactions can both read the old row, both compute a new value, and both write — and the second write silently wins. Verify this with a parallel race test (see `references/testing.md`).

### Firestore

```go
func (r TrainingsFirestoreRepository) UpdateTraining(
    ctx context.Context,
    uuid string,
    user training.User,
    updateFn func(ctx context.Context, tr *training.Training) (*training.Training, error),
) error {
    return r.client.RunTransaction(ctx, func(ctx context.Context, tx *firestore.Transaction) error {
        docRef := r.collection().Doc(uuid)
        snap, err := tx.Get(docRef)
        if err != nil {
            return err
        }
        tr, err := r.unmarshalTraining(snap)
        if err != nil {
            return err
        }
        if err := training.CanUserSeeTraining(user, *tr); err != nil {
            return err
        }
        updated, err := updateFn(ctx, tr)
        if err != nil {
            return err
        }
        return tx.Set(docRef, r.marshalTraining(updated))
    })
}
```

Firestore's transaction API is closure-shaped already, which makes this a natural fit. (Note that getting a doc transactionally vs. non-transactionally uses different APIs — extract a helper closure if the unmarshal logic is shared.)

## Secure by Design — authorization at the repository

The repository is the *only* way to reach the data. So put authorization there. A new contributor cannot accidentally skip the check.

```go
// in domain/training/user.go

func CanUserSeeTraining(user User, tr Training) error {
    if user.Type() == Trainer {
        return nil
    }
    if user.UUID() == tr.UserUUID() {
        return nil
    }
    return ForbiddenToSeeTrainingError{
        RequestingUserUUID: user.UUID(),
        TrainingOwnerUUID:  tr.UserUUID(),
    }
}
```

```go
// in adapters/trainings_firestore_repository.go

func (r TrainingsFirestoreRepository) GetTraining(
    ctx context.Context,
    uuid string,
    user training.User,
) (*training.Training, error) {
    snap, err := r.collection().Doc(uuid).Get(ctx)
    if status.Code(err) == codes.NotFound {
        return nil, training.NotFoundError{TrainingUUID: uuid}
    }
    if err != nil {
        return nil, fmt.Errorf("unable to get training: %w", err)
    }
    tr, err := r.unmarshalTraining(snap)
    if err != nil {
        return nil, err
    }
    if err := training.CanUserSeeTraining(user, *tr); err != nil {
        return nil, err
    }
    return tr, nil
}
```

The `user training.User` parameter is part of the repository's interface. There is no way to call `GetTraining` without supplying the calling user. A future contributor cannot forget — the type system won't let them.

This is the difference between "we have a test that catches that case" (which fails on the day someone introduces a new endpoint without copying the auth check) and "the code structure makes the mistake impossible to write."

## Domain-First: start with an in-memory implementation

When designing a new aggregate, you don't have to pick a database on day one. Implement `Repository` in memory with `map[string]Entity`. Write the domain layer and the application layer against it. Get unit tests green. Defer the database decision until you understand the access patterns.

Two to four weeks of this can save you from a wrong database choice. Timebox it.

## When NOT to use the repository pattern

For services that are pure read-pass-throughs (e.g. "read user record from DB and return as JSON"), the pattern adds boilerplate without much benefit. The `users` service in *Go with the Domain* deliberately does NOT use it.

A rule of thumb: if your repository would have one method (`GetByID`) and the entity is just a data carrier, you don't need a repository — you need a function.

## Common mistakes

- **Returning `*Training` from `updateFn` but mutating the input pointer.** The book recommends returning a fresh value so you can replace the entity entirely. Either way, be consistent.
- **Skipping `FOR UPDATE`.** Concurrent writes will silently overwrite each other. Always test for this — see `references/testing.md` for the race-condition test pattern.
- **Catching panics in the transaction wrapper.** Don't. Let panics propagate. The transaction will be rolled back by `defer`.
- **Defining the `Repository` interface in `adapters/` or `app/`.** It belongs in the domain package — that's how dependency inversion works in this layout.
- **Reusing DB struct tags on the domain type to "save boilerplate".** This is the trap *Go with the Domain* Chapter 5 warns against. Pay the small cost of mapping; reap the large benefit of independent evolution.
