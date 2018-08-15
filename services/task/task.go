package task

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/sirupsen/logrus"

	"github.com/lib/pq"
	nats "github.com/nats-io/go-nats"

	"github.com/jmoiron/sqlx"
	"github.com/kylycht/md/model"
	_ "github.com/lib/pq"
)

const (
	timeout = time.Second * 5
)

// Service represents Task service that will handle
// Task related DB operations
type Service struct {
	db       *sqlx.DB
	jsonConn *nats.EncodedConn
}

// NewService returns new instance of Task service
func NewService(db *sqlx.DB, conn *nats.EncodedConn) (*Service, error) {
	srv := &Service{db: db, jsonConn: conn}
	return srv, srv.init()
}

func (s *Service) init() error {
	if _, err := s.jsonConn.QueueSubscribe("task.add", "task-queue", s.New); err != nil {
		return err
	}
	if _, err := s.jsonConn.QueueSubscribe("task.get", "task-queue", s.Get); err != nil {
		return err
	}
	if _, err := s.jsonConn.QueueSubscribe("task.update", "task-queue", s.Update); err != nil {
		return err
	}
	if _, err := s.jsonConn.QueueSubscribe("task.list", "task-queue", s.List); err != nil {
		return err
	}
	if _, err := s.jsonConn.QueueSubscribe("task.delete", "task-queue", s.Delete); err != nil {
		return err
	}

	return nil
}

// New will perform DB insert operation for the given Task
func (s *Service) New(subject, reply string, t *model.Task) error {
	//get client info
	replyMsg := model.NATSMsg{}
	if err := s.jsonConn.Request("client.get", t.ClientID, &replyMsg, timeout); err != nil {
		return err
	}
	if !replyMsg.Success {
		return s.jsonConn.Publish(reply, model.NATSMsg{Success: false, Message: replyMsg.Message})
	}
	var client model.Client
	if err := json.Unmarshal(replyMsg.Data, &client); err != nil {
		return s.jsonConn.Publish(reply, model.NATSMsg{Success: false, Message: err.Error()})
	}
	// client does not have enough money to create task for Fee specified
	if client.Balance-t.Fee < 0 {
		return errors.New("Insufficient funds")
	}
	// tx begin
	tx, err := s.db.Begin()
	if err != nil {
		return s.jsonConn.Publish(reply, model.NATSMsg{Success: false, Message: err.Error()})
	}
	// withdraw client money
	balance := client.Balance - t.Fee
	withdrawFunds := "UPDATE client SET balance=$1 WHERE id=$2"
	if res, err := tx.Exec(withdrawFunds, balance, client.ID); err != nil {
		tx.Rollback()
		return s.jsonConn.Publish(reply, model.NATSMsg{Success: false, Message: err.Error()})
	} else if c, err := res.RowsAffected(); c == 0 || err != nil {
		tx.Rollback()
		return s.jsonConn.Publish(reply, model.NATSMsg{Success: false, Message: "no rows effected"})
	}

	txID := model.NewID()
	// lock funds from client account
	lockFunds := "INSERT INTO billing(id, client_id, task_id, amount, status) VALUES($1,$2,$3,$4,$5)"
	if _, err = tx.Exec(lockFunds, txID, t.ClientID, t.ID, t.Fee, model.Locked); err != nil {
		tx.Rollback()
		return s.jsonConn.Publish(reply, model.NATSMsg{Success: false, Message: err.Error()})
	}

	// create task
	if res, err := tx.Exec("INSERT INTO task (id, client_id, freelancer_id, description, fee, deadline, created_at, status) "+
		"VALUES($1, $2, $3, $4, $5, $6, $7, $8)", t.ID, t.ClientID, t.FreelancerID, t.Description, t.Fee, t.Deadline, t.CreatedAt, t.Status); err != nil {
		tx.Rollback()
		return s.jsonConn.Publish(reply, model.NATSMsg{Success: false, Message: err.Error()})
	} else if c, err := res.RowsAffected(); c == 0 || err != nil {
		tx.Rollback()
		return s.jsonConn.Publish(reply, model.NATSMsg{Success: false, Message: err.Error()})
	}
	tx.Commit()
	return s.jsonConn.Publish(reply, model.NATSMsg{Success: true})
}

func (s *Service) completeTask(subject, reply string, t *model.Task) {

}

func (s *Service) transferFunds(t *model.Task) error {
	logrus.Info("transfering funds")
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	var b sql.NullInt64
	var txID string
	if err := tx.QueryRow("SELECT id FROM billing WHERE task_id=$1", t.ID).Scan(&txID); err != nil {
		tx.Rollback()
		return err
	}
	logrus.WithField("balance", b.Int64).WithField("fee", t.Fee).Info("updating status")
	if rs, err := tx.Exec("UPDATE billing SET status=$1, paid_date=$2, freelancer_id=$3 WHERE id=$4",
		model.Paid, time.Now(), t.FreelancerID, txID); err != nil {
		tx.Rollback()
		return err
	} else if c, err := rs.RowsAffected(); c == 0 || err != nil {
		tx.Rollback()
		return err
	}
	if err := tx.QueryRow("SELECT balance FROM freelancer WHERE id=$1", t.FreelancerID).Scan(&b); err != nil {
		tx.Rollback()
		return err
	}
	logrus.WithField("balance", b.Int64).WithField("fee", t.Fee).Info("transfering funds")
	total := b.Int64 + t.Fee
	if rs, err := tx.Exec("UPDATE freelancer SET balance=$1 WHERE id=$2", total, t.FreelancerID); err != nil {
		tx.Rollback()
		return err
	} else if c, err := rs.RowsAffected(); c == 0 || err != nil {
		tx.Rollback()
		return err
	}

	tx.Commit()
	return nil
}

// Update will perform DB update operation for the given Task
// NOTE: Not all fields are updatable
// TODO: Write better query builder using reflect package
func (s *Service) Update(subject, reply string, t *model.Task) {
	var (
		updateS  strings.Builder
		position = 1
		args     []interface{}
		update   = "UPDATE task SET "
		err      error
	)
	defer func() {
		if err != nil {
			s.jsonConn.Publish(reply, model.NATSMsg{Success: false, Message: err.Error()})
			return
		}
		s.jsonConn.Publish(reply, model.NATSMsg{Success: true})
	}()

	if _, err := updateS.WriteString(update); err != nil {
		return
	}
	// check if fee is set
	if t.Fee > 0 {
		if _, err := updateS.WriteString(fmt.Sprintf("fee=$%d", position)); err != nil {
			return
		}
		position++
		args = append(args, t.Fee)
	}

	if len(t.Status) > 0 {
		if position > 1 {
			if _, err = updateS.WriteString(", "); err != nil {
				return
			}
		}
		if _, err := updateS.WriteString(fmt.Sprintf("status=$%d", position)); err != nil {
			return
		}
		switch t.Status {
		//transfer funds to freelancer
		case model.Closed:
			task, err := s.getTaskByID(t.ID)
			if err != nil {
				return
			}
			if err := s.transferFunds(&task); err != nil {
				s.jsonConn.Publish(reply, model.NATSMsg{Success: false, Message: err.Error()})
				return
			}
			//return funds to client
		case model.Abandoned:

		}
		position++
		args = append(args, t.Status)
	}

	if len(t.Description) > 0 {
		if position > 1 {
			if _, err = updateS.WriteString(", "); err != nil {
				return
			}
		}
		if _, err := updateS.WriteString(fmt.Sprintf("description=$%d", position)); err != nil {
			return
		}
		position++
		args = append(args, t.Description)
	}

	if len(t.FreelancerID) > 0 {
		if position > 1 {
			if _, err = updateS.WriteString(", "); err != nil {
				return
			}
		}
		if _, err := updateS.WriteString(fmt.Sprintf("freelancer_id=$%d", position)); err != nil {
			return
		}
		position++
		args = append(args, t.FreelancerID)
	}

	if t.Deadline > 0 {
		if position > 1 {
			if _, err = updateS.WriteString(", "); err != nil {
				return
			}
		}
		if _, err := updateS.WriteString(fmt.Sprintf("deadline=$%d", position)); err != nil {
			return
		}
		position++
		args = append(args, t.Deadline)
	}
	// nothing to update
	if update == updateS.String() {
		return
	}
	if position > 1 {
		if _, err = updateS.WriteString(", "); err != nil {
			return
		}
	}
	t.UpdatedAt = pq.NullTime{Time: time.Now(), Valid: true}
	if _, err := updateS.WriteString(fmt.Sprintf("updated_at=$%d ", position)); err != nil {
		return
	}
	position++
	args = append(args, t.UpdatedAt)

	if _, err := updateS.WriteString(fmt.Sprintf("WHERE ID=$%d", position)); err != nil {
		return
	}
	logrus.Info(updateS.String())
	args = append(args, t.ID)
	if _, err := s.db.Exec(updateS.String(), args...); err != nil {
		return
	}
}

// Delete will peform soft delete and set deleted_at datetime
func (s *Service) Delete(subject, reply, id string) error {
	if len(id) != 36 {
		return s.jsonConn.Publish(reply, &model.NATSMsg{Success: false, Message: model.ErrInvalidID.Error()})
	}
	delS := "UPDATE task SET deleted_at=$1 WHERE id=$2"
	if _, err := s.db.Exec(delS, time.Now(), id); err != nil {
		return s.jsonConn.Publish(reply, &model.NATSMsg{Success: false, Message: err.Error()})
	}
	return s.jsonConn.Publish(reply, &model.NATSMsg{Success: true})
}

// Get will perform DB select operation and retrieve Task by given ID
func (s *Service) Get(subject, reply, id string) error {
	if len(id) != 36 {
		return s.jsonConn.Publish(reply, &model.NATSMsg{Success: false, Message: model.ErrInvalidID.Error()})
	}

	task, err := s.getTaskByID(id)
	if err != nil {
		return s.jsonConn.Publish(reply, &model.NATSMsg{Success: false, Message: err.Error()})
	}

	d, err := json.Marshal(&task)
	if err != nil {
		return s.jsonConn.Publish(reply, &model.NATSMsg{Success: false, Message: err.Error()})
	}
	return s.jsonConn.Publish(reply, model.NATSMsg{Success: true, Data: d})
}

func (s *Service) getTaskByID(id string) (model.Task, error) {
	task := model.Task{}
	query := "SELECT * FROM task WHERE id = $1"
	if err := s.db.Get(&task, query, id); err != nil {
		return task, err
	}
	return task, nil
}

// List will perform DB select operation and retrieve all Tasks by give owner(client)
func (s *Service) List(subject, reply, clientID string) error {
	query := "SELECT * FROM task WHERE client_id = $1 AND deleted_at IS NULL ORDER BY created_at ASC"
	tasks := []model.Task{}
	if err := s.db.Select(&tasks, query, clientID); err != nil {
		return s.jsonConn.Publish(reply, &model.NATSMsg{Success: false, Message: err.Error()})
	}
	d, err := json.Marshal(&tasks)
	if err != nil {
		return s.jsonConn.Publish(reply, &model.NATSMsg{Success: false, Message: err.Error()})
	}
	return s.jsonConn.Publish(reply, model.NATSMsg{Success: true, Data: d})
}
