package policyassignmentrepo

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	persistdb "github.com/zivego/wiregate/internal/persistence/db"
)

// Assignment mirrors the policy_assignments table.
type Assignment struct {
	ID             string
	AgentID        string
	AccessPolicyID string
	Status         string
	CreatedAt      time.Time
}

// Repo provides policy assignment persistence operations.
type Repo struct {
	db *persistdb.Handle
}

func New(db *persistdb.Handle) *Repo {
	return &Repo{db: db}
}

func (r *Repo) Insert(ctx context.Context, assignment Assignment) error {
	_, err := r.db.ExecContext(ctx,
		`INSERT INTO policy_assignments (id, agent_id, access_policy_id, status, created_at)
		 VALUES (?, ?, ?, ?, ?)`,
		assignment.ID,
		assignment.AgentID,
		assignment.AccessPolicyID,
		assignment.Status,
		assignment.CreatedAt.UTC().Format(time.RFC3339),
	)
	if err != nil {
		return fmt.Errorf("policyassignmentrepo insert: %w", err)
	}
	return nil
}

func (r *Repo) FindActiveByAgentPolicy(ctx context.Context, agentID, policyID string) (*Assignment, error) {
	row := r.db.QueryRowContext(ctx,
		`SELECT id, agent_id, access_policy_id, status, created_at
		   FROM policy_assignments
		  WHERE agent_id = ? AND access_policy_id = ? AND status = ?
		  LIMIT 1`,
		agentID,
		policyID,
		"active",
	)
	assignment, err := scanAssignment(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("policyassignmentrepo find active by agent policy: %w", err)
	}
	return assignment, nil
}

func (r *Repo) ListByAgentID(ctx context.Context, agentID string) ([]Assignment, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT id, agent_id, access_policy_id, status, created_at
		   FROM policy_assignments
		  WHERE agent_id = ? AND status = ?
		  ORDER BY created_at ASC`,
		agentID,
		"active",
	)
	if err != nil {
		return nil, fmt.Errorf("policyassignmentrepo list by agent id: %w", err)
	}
	defer rows.Close()

	var assignments []Assignment
	for rows.Next() {
		assignment, err := scanAssignment(rows)
		if err != nil {
			return nil, fmt.Errorf("policyassignmentrepo list by agent id scan: %w", err)
		}
		assignments = append(assignments, *assignment)
	}
	return assignments, rows.Err()
}

func (r *Repo) ListByPolicyID(ctx context.Context, policyID string) ([]Assignment, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT id, agent_id, access_policy_id, status, created_at
		   FROM policy_assignments
		  WHERE access_policy_id = ? AND status = ?
		  ORDER BY created_at ASC`,
		policyID,
		"active",
	)
	if err != nil {
		return nil, fmt.Errorf("policyassignmentrepo list by policy id: %w", err)
	}
	defer rows.Close()

	var assignments []Assignment
	for rows.Next() {
		assignment, err := scanAssignment(rows)
		if err != nil {
			return nil, fmt.Errorf("policyassignmentrepo list by policy id scan: %w", err)
		}
		assignments = append(assignments, *assignment)
	}
	return assignments, rows.Err()
}

func (r *Repo) ListByPolicyIDs(ctx context.Context, policyIDs []string) ([]Assignment, error) {
	if len(policyIDs) == 0 {
		return nil, nil
	}

	placeholders := make([]string, 0, len(policyIDs))
	args := make([]any, 0, len(policyIDs)+1)
	for _, policyID := range policyIDs {
		if strings.TrimSpace(policyID) == "" {
			continue
		}
		placeholders = append(placeholders, "?")
		args = append(args, policyID)
	}
	if len(placeholders) == 0 {
		return nil, nil
	}
	args = append(args, "active")

	rows, err := r.db.QueryContext(ctx,
		`SELECT id, agent_id, access_policy_id, status, created_at
		   FROM policy_assignments
		  WHERE access_policy_id IN (`+strings.Join(placeholders, ", ")+`) AND status = ?
		  ORDER BY created_at ASC`,
		args...,
	)
	if err != nil {
		return nil, fmt.Errorf("policyassignmentrepo list by policy ids: %w", err)
	}
	defer rows.Close()

	var assignments []Assignment
	for rows.Next() {
		assignment, err := scanAssignment(rows)
		if err != nil {
			return nil, fmt.Errorf("policyassignmentrepo list by policy ids scan: %w", err)
		}
		assignments = append(assignments, *assignment)
	}
	return assignments, rows.Err()
}

func (r *Repo) DeactivateActiveByAgentPolicy(ctx context.Context, agentID, policyID string) (bool, error) {
	result, err := r.db.ExecContext(ctx,
		`UPDATE policy_assignments
		    SET status = ?
		  WHERE agent_id = ? AND access_policy_id = ? AND status = ?`,
		"inactive",
		agentID,
		policyID,
		"active",
	)
	if err != nil {
		return false, fmt.Errorf("policyassignmentrepo deactivate active by agent policy: %w", err)
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("policyassignmentrepo deactivate active by agent policy rows affected: %w", err)
	}
	return affected > 0, nil
}

type scanner interface {
	Scan(dest ...any) error
}

func scanAssignment(row scanner) (*Assignment, error) {
	var assignment Assignment
	var createdAt string
	err := row.Scan(&assignment.ID, &assignment.AgentID, &assignment.AccessPolicyID, &assignment.Status, &createdAt)
	if err != nil {
		return nil, err
	}
	assignment.CreatedAt, _ = time.Parse(time.RFC3339, createdAt)
	return &assignment, nil
}
