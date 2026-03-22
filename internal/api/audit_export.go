package api

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"

	"github.com/zivego/wiregate/internal/audit"
)

type auditExportRequest struct {
	Destination  string `json:"destination,omitempty"`
	Path         string `json:"path,omitempty"`
	Bucket       string `json:"bucket,omitempty"`
	Key          string `json:"key,omitempty"`
	Region       string `json:"region,omitempty"`
	Action       string `json:"action,omitempty"`
	ResourceType string `json:"resource_type,omitempty"`
	Result       string `json:"result,omitempty"`
	ActorUserID  string `json:"actor_user_id,omitempty"`
}

type auditExportResponse struct {
	Format      string `json:"format"`
	Destination string `json:"destination"`
	Path        string `json:"path,omitempty"`
	Bucket      string `json:"bucket,omitempty"`
	Key         string `json:"key,omitempty"`
	Region      string `json:"region,omitempty"`
	Events      int    `json:"events"`
	Bytes       int64  `json:"bytes"`
}

func (r *Router) handleExportAuditEvents(w http.ResponseWriter, req *http.Request) {
	claims, ok := r.authenticate(w, req)
	if !ok {
		return
	}
	if claims.Role != "admin" {
		writeError(w, http.StatusForbidden, "forbidden", "admin role required")
		return
	}
	if !r.limitSensitiveAction(w, req, claims.UserID, "audit.export") {
		return
	}

	var body auditExportRequest
	if err := r.decodeJSONBody(w, req, &body); err != nil {
		writeDecodeError(w, err)
		return
	}
	destination := strings.ToLower(strings.TrimSpace(body.Destination))
	if destination == "" {
		destination = "file"
	}
	if destination != "file" && destination != "s3" {
		writeError(w, http.StatusBadRequest, "validation_failed", "destination must be file or s3")
		return
	}

	tmpFile, err := os.CreateTemp("", "wiregate-audit-export-*.ndjson")
	if err != nil {
		r.logger.Printf("audit export create temp file error: %v", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "failed to prepare export")
		return
	}
	tmpPath := tmpFile.Name()
	removeTemp := true
	defer func() {
		if removeTemp {
			_ = os.Remove(tmpPath)
		}
	}()

	eventsCount, bytesWritten, err := r.auditService.ExportNDJSON(req.Context(), tmpFile, audit.ListFilter{
		Action:       strings.TrimSpace(body.Action),
		ResourceType: strings.TrimSpace(body.ResourceType),
		Result:       strings.TrimSpace(body.Result),
		ActorUserID:  strings.TrimSpace(body.ActorUserID),
	})
	if err != nil {
		r.logger.Printf("audit export stream error: %v", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "failed to export audit events")
		_ = tmpFile.Close()
		return
	}
	if err := tmpFile.Close(); err != nil {
		r.logger.Printf("audit export close temp file error: %v", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "failed to finalize export")
		return
	}

	resp := auditExportResponse{
		Format:      "ndjson",
		Destination: destination,
		Events:      eventsCount,
		Bytes:       bytesWritten,
	}

	switch destination {
	case "file":
		targetPath, err := resolveAuditExportTargetPath(r.auditExport.Directory, body.Path)
		if err != nil {
			writeError(w, http.StatusBadRequest, "validation_failed", err.Error())
			return
		}
		if err := moveFileAtomic(tmpPath, targetPath); err != nil {
			r.logger.Printf("audit export move file error: %v", err)
			writeError(w, http.StatusInternalServerError, "internal_error", "failed to write export file")
			return
		}
		removeTemp = false
		resp.Path = targetPath
	case "s3":
		region := strings.TrimSpace(body.Region)
		if region == "" {
			region = strings.TrimSpace(r.auditExport.S3Region)
		}
		bucket := strings.TrimSpace(body.Bucket)
		if bucket == "" {
			bucket = strings.TrimSpace(r.auditExport.S3Bucket)
		}
		key := strings.TrimSpace(body.Key)
		if key == "" {
			key = strings.TrimSpace(body.Path)
		}
		if key == "" {
			key = "audit-export-" + time.Now().UTC().Format("20060102T150405Z") + ".ndjson"
		}
		if prefix := strings.Trim(strings.TrimSpace(r.auditExport.S3Prefix), "/"); prefix != "" {
			key = strings.Trim(prefix+"/"+strings.TrimLeft(key, "/"), "/")
		}
		if bucket == "" {
			writeError(w, http.StatusBadRequest, "validation_failed", "bucket is required for s3 export")
			return
		}
		if region == "" {
			writeError(w, http.StatusBadRequest, "validation_failed", "region is required for s3 export")
			return
		}
		if err := uploadAuditExportToS3(req.Context(), region, bucket, key, tmpPath, r.auditExport.S3Endpoint, r.auditExport.S3Insecure); err != nil {
			r.logger.Printf("audit export s3 upload error: %v", err)
			writeError(w, http.StatusInternalServerError, "internal_error", "failed to upload export to s3")
			return
		}
		resp.Bucket = bucket
		resp.Key = key
		resp.Region = region
	}

	r.recordAuditEvent(req.Context(), audit.Event{
		ActorUserID:  claims.UserID,
		Action:       "audit.export",
		ResourceType: "audit_event",
		Result:       "success",
		Metadata: map[string]any{
			"destination": destination,
			"events":      eventsCount,
			"bytes":       bytesWritten,
			"path":        resp.Path,
			"bucket":      resp.Bucket,
			"key":         resp.Key,
			"region":      resp.Region,
		},
	})
	writeJSON(w, http.StatusOK, resp)
}

func resolveAuditExportTargetPath(baseDir, requestedPath string) (string, error) {
	if strings.TrimSpace(baseDir) == "" {
		baseDir = "./data/exports/audit"
	}
	baseAbs, err := filepath.Abs(filepath.Clean(baseDir))
	if err != nil {
		return "", fmt.Errorf("invalid export directory")
	}

	cleanRequested := strings.TrimSpace(requestedPath)
	if cleanRequested == "" {
		cleanRequested = "audit-export-" + time.Now().UTC().Format("20060102T150405Z") + ".ndjson"
	}
	if filepath.IsAbs(cleanRequested) {
		return "", fmt.Errorf("path must be relative")
	}
	cleanRequested = filepath.Clean(cleanRequested)
	if cleanRequested == "." || strings.HasPrefix(cleanRequested, ".."+string(os.PathSeparator)) || cleanRequested == ".." {
		return "", fmt.Errorf("path must stay within the export directory")
	}
	if !strings.HasSuffix(strings.ToLower(cleanRequested), ".ndjson") {
		cleanRequested += ".ndjson"
	}

	target := filepath.Join(baseAbs, cleanRequested)
	targetAbs, err := filepath.Abs(target)
	if err != nil {
		return "", fmt.Errorf("invalid export path")
	}
	if targetAbs != baseAbs && !strings.HasPrefix(targetAbs, baseAbs+string(os.PathSeparator)) {
		return "", fmt.Errorf("path must stay within the export directory")
	}

	if err := os.MkdirAll(filepath.Dir(targetAbs), 0o755); err != nil {
		return "", fmt.Errorf("failed to create export directory: %w", err)
	}
	return targetAbs, nil
}

func moveFileAtomic(src, dst string) error {
	if err := os.Rename(src, dst); err == nil {
		return nil
	}
	return copyFile(src, dst)
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		_ = out.Close()
		return err
	}
	if err := out.Sync(); err != nil {
		_ = out.Close()
		return err
	}
	if err := out.Close(); err != nil {
		return err
	}
	return os.Remove(src)
}

func uploadAuditExportToS3(ctx context.Context, region, bucket, key, path, endpoint string, insecure bool) error {
	endpoint = strings.TrimSpace(endpoint)
	if endpoint == "" {
		if strings.EqualFold(region, "us-east-1") {
			endpoint = "s3.amazonaws.com"
		} else {
			endpoint = "s3." + region + ".amazonaws.com"
		}
	}
	client, err := minio.New(endpoint, &minio.Options{
		Creds:  credentials.NewEnvAWS(),
		Secure: !insecure,
		Region: region,
	})
	if err != nil {
		return err
	}
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return err
	}

	_, err = client.PutObject(ctx, bucket, key, file, info.Size(), minio.PutObjectOptions{
		ContentType: "application/x-ndjson",
	})
	return err
}
