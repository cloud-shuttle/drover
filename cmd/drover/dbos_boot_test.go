package main

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/cloud-shuttle/drover-libs/pkg/dbosboot"
	"github.com/dbos-inc/dbos-transact-golang/dbos"
	"github.com/jackc/pgx/v5"
)

func TestDBOSOptionsCapped(t *testing.T) {
	t.Setenv("DROVER_PG_POOLER", "")
	cases := []struct {
		url  string
		port uint16
		max  int32
	}{
		{"postgres://postgres:postgres@localhost:5432/drover?sslmode=disable", 5432, dbosboot.DefaultMaxConns},
		// DBOS must never go through PgBouncer, even if the URL says 6432.
		{"postgres://u:p@aws-ap-southeast-2-1.pg.psdb.cloud:6432/drover?sslmode=require", 5432, dbosboot.DefaultMaxConns},
		{"postgres://u:p@localhost:5432/drover?pool_max_conns=8", 5432, 8},
	}
	for _, tc := range cases {
		o := dbosOptions("drover", tc.url)
		if !o.EnsureDatabase || o.ApplicationName != "drover" || o.AppName != "drover" {
			t.Fatalf("options %+v", o)
		}
		cfg, err := dbosboot.PoolConfig(o)
		if err != nil {
			t.Fatal(err)
		}
		if cfg.ConnConfig.Port != tc.port || cfg.MaxConns != tc.max {
			t.Fatalf("%s: port %d max %d, want %d and %d", tc.url, cfg.ConnConfig.Port, cfg.MaxConns, tc.port, tc.max)
		}
		if cfg.MaxConnLifetime != time.Hour || cfg.MaxConnIdleTime != 5*time.Minute || cfg.ConnConfig.ConnectTimeout != 10*time.Second {
			t.Fatalf("tuning %v %v %v", cfg.MaxConnLifetime, cfg.MaxConnIdleTime, cfg.ConnConfig.ConnectTimeout)
		}
	}
}

func TestNewDBOSContextRejectsBadURL(t *testing.T) {
	if _, _, err := newDBOSContext(context.Background(), "drover", "postgres://%zz"); err == nil {
		t.Fatal("want error")
	}
}

// Runs when DBOS_SYSTEM_DATABASE_URL is set; uses a fresh database to check
// the CLI still creates a missing system database like DBOS did on its own.
func TestNewDBOSContextIntegration(t *testing.T) {
	base := os.Getenv("DBOS_SYSTEM_DATABASE_URL")
	if base == "" {
		t.Skip("DBOS_SYSTEM_DATABASE_URL not set")
	}
	cc, err := pgx.ParseConfig(base)
	if err != nil {
		t.Fatal(err)
	}
	name := fmt.Sprintf("drover_boot_%d", time.Now().UnixNano())
	url := fmt.Sprintf("postgres://%s:%s@%s:%d/%s?sslmode=disable", cc.User, cc.Password, cc.Host, cc.Port, name)
	t.Cleanup(func() {
		admin := cc.Copy()
		admin.Database = "postgres"
		if conn, err := pgx.ConnectConfig(context.Background(), admin); err == nil {
			_, _ = conn.Exec(context.Background(), "DROP DATABASE IF EXISTS "+pgx.Identifier{name}.Sanitize()+" WITH (FORCE)")
			_ = conn.Close(context.Background())
		}
	})
	dbosCtx, pool, err := newDBOSContext(context.Background(), "drover", url)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	if err := dbos.Launch(dbosCtx); err != nil {
		t.Fatal(err)
	}
	dbos.Shutdown(dbosCtx, 5*time.Second)
	if pool.Config().MaxConns != dbosboot.DefaultMaxConns {
		t.Fatalf("MaxConns %d", pool.Config().MaxConns)
	}
}
