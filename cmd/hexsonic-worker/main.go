package main

import (
	"context"
	"errors"
	"log"
	"path/filepath"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"hexsonic/internal/config"
	"hexsonic/internal/media"
	"hexsonic/internal/storage"
)

type job struct {
	ID       int64
	TrackID  string
	FilePath string
}

func main() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("config: %v", err)
	}
	pool, err := pgxpool.New(context.Background(), cfg.DatabaseURL)
	if err != nil {
		log.Fatalf("database: %v", err)
	}
	defer pool.Close()
	store, err := storage.New(cfg.StorageRoot)
	if err != nil {
		log.Fatalf("storage: %v", err)
	}

	log.Println("HEXSONIC worker started")
	for {
		j, ok, err := claimJob(context.Background(), pool)
		if err != nil {
			log.Printf("claim job error: %v", err)
			time.Sleep(2 * time.Second)
			continue
		}
		if !ok {
			time.Sleep(1 * time.Second)
			continue
		}
		if err := processJob(context.Background(), cfg, store, j); err != nil {
			log.Printf("job %d failed: %v", j.ID, err)
			if markErr := markJobFailed(context.Background(), pool, j.ID, err.Error()); markErr != nil {
				log.Printf("mark job failed error: %v", markErr)
			}
			continue
		}
		if err := markJobDone(context.Background(), pool, j.ID); err != nil {
			log.Printf("mark job done error: %v", err)
		}
	}
}

func claimJob(ctx context.Context, pool *pgxpool.Pool) (job, bool, error) {
	tx, err := pool.Begin(ctx)
	if err != nil {
		return job{}, false, err
	}
	defer tx.Rollback(ctx)

	var j job
	err = tx.QueryRow(ctx, `
		SELECT j.id, j.track_id::text, tf.file_path
		FROM transcode_jobs j
		JOIN track_files tf ON tf.id = j.source_file_id
		WHERE j.status = 'queued'
		ORDER BY j.created_at
		FOR UPDATE SKIP LOCKED
		LIMIT 1
	`).Scan(&j.ID, &j.TrackID, &j.FilePath)
	if errors.Is(err, pgx.ErrNoRows) {
		return job{}, false, tx.Commit(ctx)
	}
	if err != nil {
		return job{}, false, err
	}

	if _, err := tx.Exec(ctx, `
		UPDATE transcode_jobs
		SET status='processing', attempts=attempts+1, locked_at=now(), updated_at=now()
		WHERE id=$1
	`, j.ID); err != nil {
		return job{}, false, err
	}

	if err := tx.Commit(ctx); err != nil {
		return job{}, false, err
	}
	return j, true, nil
}

func processJob(ctx context.Context, cfg config.Config, store *storage.Store, j job) error {
	ctx, cancel := context.WithTimeout(ctx, 2*time.Hour)
	defer cancel()
	dir := store.DerivedTrackDir(j.TrackID)
	if err := media.TranscodeMP3(ctx, cfg.FFmpegBin, j.FilePath, filepath.Join(dir, "320.mp3")); err != nil {
		return err
	}
	if err := media.TranscodeAAC(ctx, cfg.FFmpegBin, j.FilePath, filepath.Join(dir, "mobile.m4a")); err != nil {
		return err
	}
	if err := media.TranscodeOpus(ctx, cfg.FFmpegBin, j.FilePath, filepath.Join(dir, "opus160.opus")); err != nil {
		return err
	}
	if err := media.BuildWaveform(ctx, cfg.FFmpegBin, j.FilePath, filepath.Join(dir, "waveform.json")); err != nil {
		return err
	}
	return nil
}

func markJobDone(ctx context.Context, pool *pgxpool.Pool, id int64) error {
	_, err := pool.Exec(ctx, `
		UPDATE transcode_jobs
		SET status='done', updated_at=now(), error_text=NULL
		WHERE id=$1
	`, id)
	return err
}

func markJobFailed(ctx context.Context, pool *pgxpool.Pool, id int64, reason string) error {
	_, err := pool.Exec(ctx, `
		UPDATE transcode_jobs
		SET status='failed', updated_at=now(), error_text=$2
		WHERE id=$1
	`, id, reason)
	return err
}
