//! Durable execution - checkpointing and state persistence

use std::path::Path;
use sqlx::{Pool, Postgres, Sqlite, sqlite::SqlitePool, postgres::PgPool};
use uuid::Uuid;
use anyhow::Result;
use crate::drover::{WorkManifest, DroverResult};

/// Durable store for checkpointing run state
pub enum DurableStore {
    Sqlite(SqliteStore),
    Postgres(PostgresStore),
}

impl DurableStore {
    /// Create a new store from a database URL
    pub async fn connect(database_url: &str) -> Result<Self> {
        if database_url.starts_with("sqlite://") {
            let path = database_url.trim_start_matches("sqlite://");
            SqliteStore::new(path).await.map(DurableStore::Sqlite)
        } else if database_url.starts_with("postgres://") || database_url.starts_with("postgresql://") {
            PostgresStore::new(database_url).await.map(DurableStore::Postgres)
        } else {
            anyhow::bail!("Unsupported database URL: {}", database_url);
        }
    }

    /// Initialize the database schema
    pub async fn init(&self) -> Result<()> {
        match self {
            DurableStore::Sqlite(s) => s.init().await,
            DurableStore::Postgres(p) => p.init().await,
        }
    }

    /// Start a new run, recording initial state
    pub async fn start_run(&self, run_id: &Uuid, manifest: &WorkManifest) -> Result<()> {
        match self {
            DurableStore::Sqlite(s) => s.start_run(run_id, manifest).await,
            DurableStore::Postgres(p) => p.start_run(run_id, manifest).await,
        }
    }

    /// Record run completion
    pub async fn complete_run(&self, run_id: &Uuid, result: &DroverResult) -> Result<()> {
        match self {
            DurableStore::Sqlite(s) => s.complete_run(run_id, result).await,
            DurableStore::Postgres(p) => p.complete_run(run_id, result).await,
        }
    }

    /// List all runs
    pub async fn list_runs(&self) -> Result<Vec<RunRecord>> {
        match self {
            DurableStore::Sqlite(s) => s.list_runs().await,
            DurableStore::Postgres(p) => p.list_runs().await,
        }
    }

    /// Get a specific run
    pub async fn get_run(&self, run_id: &Uuid) -> Result<Option<RunRecord>> {
        match self {
            DurableStore::Sqlite(s) => s.get_run(run_id).await,
            DurableStore::Postgres(p) => p.get_run(run_id).await,
        }
    }
}

/// Record of a Drover run
#[derive(Debug, Clone)]
pub struct RunRecord {
    pub id: Uuid,
    pub started_at: chrono::DateTime<chrono::Utc>,
    pub completed_at: Option<chrono::DateTime<chrono::Utc>>,
    pub success: Option<bool>,
    pub tasks_total: i32,
    pub tasks_completed: i32,
    pub tasks_failed: i32,
}

// ============================================================================
// SQLITE IMPLEMENTATION
// ============================================================================

pub struct SqliteStore {
    pool: SqlitePool,
}

impl SqliteStore {
    pub async fn new(path: &str) -> Result<Self> {
        let pool = SqlitePool::connect(path).await?;
        Ok(Self { pool })
    }

    async fn init(&self) -> Result<()> {
        sqlx::query(
            r#"
            CREATE TABLE IF NOT EXISTS runs (
                id TEXT PRIMARY KEY,
                started_at TEXT NOT NULL,
                completed_at TEXT,
                success INTEGER,
                tasks_total INTEGER NOT NULL,
                tasks_completed INTEGER NOT NULL,
                tasks_failed INTEGER NOT NULL,
                manifest TEXT NOT NULL
            )
            "#,
        )
        .execute(&self.pool)
        .await?;

        Ok(())
    }

    async fn start_run(&self, run_id: &Uuid, manifest: &WorkManifest) -> Result<()> {
        let manifest_json = serde_json::to_string(manifest)?;

        sqlx::query(
            r#"
            INSERT INTO runs (id, started_at, tasks_total, tasks_completed, tasks_failed, manifest)
            VALUES (?1, ?2, ?3, ?4, ?5, ?6)
            "#,
        )
        .bind(run_id.to_string())
        .bind(chrono::Utc::now().to_rfc3339())
        .bind(manifest.total_tasks as i32)
        .bind(0)
        .bind(0)
        .bind(manifest_json)
        .execute(&self.pool)
        .await?;

        tracing::info!("Started run {}", run_id);
        Ok(())
    }

    async fn complete_run(&self, run_id: &Uuid, result: &DroverResult) -> Result<()> {
        sqlx::query(
            r#"
            UPDATE runs
            SET completed_at = ?1,
                success = ?2,
                tasks_completed = ?3,
                tasks_failed = ?4
            WHERE id = ?5
            "#,
        )
        .bind(chrono::Utc::now().to_rfc3339())
        .bind(result.success)
        .bind(result.tasks_completed as i32)
        .bind(result.tasks_failed as i32)
        .bind(run_id.to_string())
        .execute(&self.pool)
        .await?;

        tracing::info!("Completed run {}", run_id);
        Ok(())
    }

    async fn list_runs(&self) -> Result<Vec<RunRecord>> {
        let rows = sqlx::query_as::<_, (String, String, Option<String>, Option<bool>, i32, i32, i32)>(
            "SELECT id, started_at, completed_at, success, tasks_total, tasks_completed, tasks_failed FROM runs ORDER BY started_at DESC"
        )
        .fetch_all(&self.pool)
        .await?;

        Ok(rows.into_iter().map(|(id, started, completed, success, total, completed_cnt, failed)| {
            RunRecord {
                id: Uuid::parse_str(&id).unwrap_or_default(),
                started_at: chrono::DateTime::parse_from_rfc3339(&started)
                    .unwrap()
                    .with_timezone(&chrono::Utc),
                completed_at: completed.and_then(|s|
                    chrono::DateTime::parse_from_rfc3339(&s).ok()
                ).map(|dt| dt.with_timezone(&chrono::Utc)),
                success,
                tasks_total: total,
                tasks_completed: completed_cnt,
                tasks_failed: failed,
            }
        }).collect())
    }

    async fn get_run(&self, run_id: &Uuid) -> Result<Option<RunRecord>> {
        let row = sqlx::query_as::<_, (String, String, Option<String>, Option<bool>, i32, i32, i32)>(
            "SELECT id, started_at, completed_at, success, tasks_total, tasks_completed, tasks_failed FROM runs WHERE id = ?1"
        )
        .bind(run_id.to_string())
        .fetch_optional(&self.pool)
        .await?;

        Ok(row.map(|(id, started, completed, success, total, completed_cnt, failed)| {
            RunRecord {
                id: Uuid::parse_str(&id).unwrap_or_default(),
                started_at: chrono::DateTime::parse_from_rfc3339(&started)
                    .unwrap()
                    .with_timezone(&chrono::Utc),
                completed_at: completed.and_then(|s|
                    chrono::DateTime::parse_from_rfc3339(&s).ok()
                ).map(|dt| dt.with_timezone(&chrono::Utc)),
                success,
                tasks_total: total,
                tasks_completed: completed_cnt,
                tasks_failed: failed,
            }
        }))
    }
}

// ============================================================================
// POSTGRES IMPLEMENTATION
// ============================================================================

pub struct PostgresStore {
    pool: PgPool,
}

impl PostgresStore {
    pub async fn new(url: &str) -> Result<Self> {
        let pool = PgPool::connect(url).await?;
        Ok(Self { pool })
    }

    async fn init(&self) -> Result<()> {
        sqlx::query(
            r#"
            CREATE TABLE IF NOT EXISTS runs (
                id UUID PRIMARY KEY,
                started_at TIMESTAMPTZ NOT NULL,
                completed_at TIMESTAMPTZ,
                success BOOLEAN,
                tasks_total INTEGER NOT NULL,
                tasks_completed INTEGER NOT NULL,
                tasks_failed INTEGER NOT NULL,
                manifest JSONB NOT NULL
            )
            "#,
        )
        .execute(&self.pool)
        .await?;

        Ok(())
    }

    async fn start_run(&self, run_id: &Uuid, manifest: &WorkManifest) -> Result<()> {
        let manifest_json = serde_json::to_value(manifest)?;

        sqlx::query(
            r#"
            INSERT INTO runs (id, started_at, tasks_total, tasks_completed, tasks_failed, manifest)
            VALUES ($1, $2, $3, $4, $5, $6)
            "#,
        )
        .bind(run_id)
        .bind(chrono::Utc::now())
        .bind(manifest.total_tasks as i32)
        .bind(0)
        .bind(0)
        .bind(manifest_json)
        .execute(&self.pool)
        .await?;

        tracing::info!("Started run {}", run_id);
        Ok(())
    }

    async fn complete_run(&self, run_id: &Uuid, result: &DroverResult) -> Result<()> {
        sqlx::query(
            r#"
            UPDATE runs
            SET completed_at = $1,
                success = $2,
                tasks_completed = $3,
                tasks_failed = $4
            WHERE id = $5
            "#,
        )
        .bind(chrono::Utc::now())
        .bind(result.success)
        .bind(result.tasks_completed as i32)
        .bind(result.tasks_failed as i32)
        .bind(run_id)
        .execute(&self.pool)
        .await?;

        tracing::info!("Completed run {}", run_id);
        Ok(())
    }

    async fn list_runs(&self) -> Result<Vec<RunRecord>> {
        let rows = sqlx::query_as::<_, (Uuid, chrono::DateTime<chrono::Utc>, Option<chrono::DateTime<chrono::Utc>>, Option<bool>, i32, i32, i32)>(
            "SELECT id, started_at, completed_at, success, tasks_total, tasks_completed, tasks_failed FROM runs ORDER BY started_at DESC"
        )
        .fetch_all(&self.pool)
        .await?;

        Ok(rows.into_iter().map(|(id, started, completed, success, total, completed_cnt, failed)| {
            RunRecord {
                id,
                started_at: started,
                completed_at: completed,
                success,
                tasks_total: total,
                tasks_completed: completed_cnt,
                tasks_failed: failed,
            }
        }).collect())
    }

    async fn get_run(&self, run_id: &Uuid) -> Result<Option<RunRecord>> {
        let row = sqlx::query_as::<_, (Uuid, chrono::DateTime<chrono::Utc>, Option<chrono::DateTime<chrono::Utc>>, Option<bool>, i32, i32, i32)>(
            "SELECT id, started_at, completed_at, success, tasks_total, tasks_completed, tasks_failed FROM runs WHERE id = $1"
        )
        .bind(run_id)
        .fetch_optional(&self.pool)
        .await?;

        Ok(row.map(|(id, started, completed, success, total, completed_cnt, failed)| {
            RunRecord {
                id,
                started_at: started,
                completed_at: completed,
                success,
                tasks_total: total,
                tasks_completed: completed_cnt,
                tasks_failed: failed,
            }
        }))
    }
}
