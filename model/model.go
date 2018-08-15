package model

import (
	"database/sql"
	"encoding/json"
	"errors"
	"time"

	"github.com/lib/pq"
	uuid "github.com/satori/go.uuid"
)

var (
	// ErrInvalidID represents error message return on invalid identifier
	ErrInvalidID = errors.New("invalid ID")
)

// TaskStatus represents status of the Task
type TaskStatus string

// PaymentStatus represents current status of the Payment
type PaymentStatus string

const (
	// Open status means that Task was successfully created and open for applications
	Open = TaskStatus("open")
	// Started status means that Freelancer has started working on the Task
	Started = TaskStatus("started")
	// Completed status means that Task was completed
	Completed = TaskStatus("completed")
	// Closed status means that Task was closed(after client review)
	Closed = TaskStatus("closed")
	// Abandoned status means that Task was abandoned by freelancer
	Abandoned = TaskStatus("abondoned")
)

const (
	// Locked status represents that funds for the task from account were locked
	Locked = PaymentStatus("locked")
	// Pending status represents that payment is in progress
	Pending = PaymentStatus("pending")
	// Canceled status represents that payment was canceled
	Canceled = PaymentStatus("canceled")
	// Loaded status represents that payment was processed and transfered to account
	Loaded = PaymentStatus("loaded")
	// Paid
	Paid = PaymentStatus("paid")
)

type (
	// Task represents a job that can be performed on job-exchange
	Task struct {
		ID           string        `db:"id"`                                 // ID represents task unique identifier(UUID)
		ClientID     string        `db:"client_id" json:"client_id"`         // ClientID represents owner ID
		FreelancerID string        `db:"freelancer_id" json:"freelancer_id"` // FreelancerID represents Freelancer's ID
		Description  string        `db:"description"`                        // Description represents description of the Task
		Fee          int64         `db:"fee"`                                // Fee represents amount to be paid upon completion of the Task
		Deadline     time.Duration `db:"deadline"`                           // Deadline represents duration of the Task
		Status       TaskStatus    `db:"status"`                             // TaskStatus represents current status of the Task
		StartedAt    pq.NullTime   `db:"started_at"`                         // StartedAt represents datetime when Freelancer started the Task
		DeletedAt    pq.NullTime   `db:"deleted_at"`                         // DeletedAt represents datetime when the Task was deleted(soft delete)
		UpdatedAt    pq.NullTime   `db:"updated_at"`                         // UpdatedAt represents last updated datetime of the Task
		CreatedAt    time.Time     `db:"created_at"`                         // CreatedAt represents datetime when Client created the Task
	}

	// Freelancer represents a freelancer(obviously)
	Freelancer struct {
		ID          string         `db:"id"`          // ID represents Freelnacer's unique identifier
		Description sql.NullString `db:"description"` // Description of the Freelancer's relative work experience
		Details     sql.NullString `db:"details"`     // Details of the Freelancer
		Email       string         `db:"email"`       // Email of the Freelancer
		Balance     sql.NullInt64  `db:"balance"`     // Balance is amount of money freelancer possess
		DeletedAt   pq.NullTime    `db:"deleted_at"`  // DeletedAt represents datetime when the Task was deleted(soft delete)
	}

	// Payment reprents payment for the Task performed by Freelancer
	Payment struct {
		ID           string        `db:"id"`            // Payment transaction indentifier
		ClientID     string        `db:"client_id"`     // ClientID represents Client's ID
		FreelancerID string        `db:"freelancer_id"` // FreelancerID represents Freelancer's ID
		TaskID       string        `db:"task_id"`       // TaskID represents Task's ID
		Amount       int64         `db:"amount"`        // Amount represents amount to be paid in cents
		PaidDate     time.Time     `db:"paid_date"`     // PaidDate  represents datetime when payment was processed
		Status       PaymentStatus `db:"status"`        // PaymentStatus represents current status of the payment
	}

	// Client represents individual client or organization
	Client struct {
		ID        string      `db:"id"`         // ID represents Client's unique identifier
		Email     string      `db:"email"`      // Email represents Client's email
		Balance   int64       `db:"balance"`    // Balance represents amount of money left on the account in cents
		DeletedAt pq.NullTime `db:"deleted_at"` // DeletedAt represents datetime when the Client was deleted(soft delete)
	}

	// NATSMsg represents message used for request/response via NATS
	NATSMsg struct {
		Success bool            `json:"success"`
		Message string          `json:"message,omitempty"`
		Data    json.RawMessage `json:"data,omitempty"`
	}
)

// NewTask is a helper func to create new Task struct
func NewTask(deadline time.Duration, fee int64, clientID, description string) Task {
	return Task{
		ID:          NewID(),
		CreatedAt:   time.Now(),
		Status:      Open,
		Fee:         fee,
		ClientID:    clientID,
		Description: description,
		Deadline:    deadline,
	}
}

// NewClient is a helper func to create new Client struct
func NewClient(email string, balance int64) Client {
	return Client{
		ID:      NewID(),
		Email:   email,
		Balance: balance,
	}
}

// NewFreelancer is a helper func to create new Freelancer struct
func NewFreelancer(email, description, details string) Freelancer {
	return Freelancer{
		ID:          NewID(),
		Email:       email,
		Description: sql.NullString{String: description, Valid: true},
		Details:     sql.NullString{String: details, Valid: true},
	}
}

// NewID is a wrapper around go.uuid.NewV4() func to supress possible error
func NewID() string {
	if i, err := uuid.NewV4(); err == nil {
		return i.String()
	}
	return ""
}
