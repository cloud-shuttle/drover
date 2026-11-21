package main

import (
	"context"
	"time"

	"github.com/cloud-shuttle/drover-libs/pkg/dbosboot"
	"github.com/dbos-inc/dbos-transact-golang/dbos"
	"github.com/jackc/pgx/v5/pgxpool"
)

// dbosOptions are the drover CLI's drover-libs dbosboot options. The system
// pool is a direct Session pool (never PgBouncer: DBOS uses LISTEN/NOTIFY and
// runs its migrations on it) capped at dbosboot.DefaultMaxConns (3) instead
// of the SDK's own 20; pool_max_conns in DBOS_SYSTEM_DATABASE_URL raises it.
// The rest mirrors what DBOS does when it opens its own pool, so local runs
// behave as before: create the database if it is missing, application_name
// = AppName, 1h lifetime, 5m idle, 10s connect timeout.
func dbosOptions(appName, databaseURL string) dbosboot.Options {
	return dbosboot.Options{
		AppName:           appName,
		SystemDatabaseURL: databaseURL,
		ApplicationName:   appName,
		MaxConnLifetime:   time.Hour,
		MaxConnIdleTime:   5 * time.Minute,
		ConnectTimeout:    10 * time.Second,
		EnsureDatabase:    true,
	}
}

// newDBOSContext opens the capped system pool and a DBOS context on it. The
// caller must call dbos.Shutdown and then close the returned pool.
func newDBOSContext(ctx context.Context, appName, databaseURL string) (dbos.DBOSContext, *pgxpool.Pool, error) {
	cfg, pool, err := dbosboot.Config(ctx, dbosOptions(appName, databaseURL))
	if err != nil {
		return nil, nil, err
	}
	dbosCtx, err := dbos.NewDBOSContext(ctx, cfg)
	if err != nil {
		pool.Close()
		return nil, nil, err
	}
	return dbosCtx, pool, nil
}
