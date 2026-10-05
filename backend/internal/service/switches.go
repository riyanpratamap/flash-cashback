package service

import (
	"context"

	"github.com/jackc/pgx/v5"

	"github.com/riyanpratamap/flash-cashback/backend/internal/domain"
	"github.com/riyanpratamap/flash-cashback/backend/internal/store"
)

// Switches is the operator use case: pause or resume awards or redemptions.
type Switches struct {
	tx store.TxRunner
}

// NewSwitches builds Switches on the transaction runner.
func NewSwitches(tx store.TxRunner) *Switches { return &Switches{tx: tx} }

// SwitchChange is a committed switch command.
type SwitchChange struct {
	Switch   domain.Switch
	Action   string // "pause" or "resume"
	Operator string
	Old, New bool
	Changed  bool
	At       string // the transaction's now(), WIB RFC 3339
}

// Set moves sw to paused in one transaction. It returns only after COMMIT, so
// a caller that reports the change reports a committed one.
func (s *Switches) Set(ctx context.Context, sw domain.Switch, paused bool, operator string) (SwitchChange, error) {
	var out store.SwitchOutcome
	err := s.tx.InTx(ctx, func(ctx context.Context, tx pgx.Tx) error {
		var err error
		out, err = store.SetSwitch(ctx, tx, campaignID, sw, paused)
		return err
	})
	if err != nil {
		return SwitchChange{}, err
	}
	return SwitchChange{
		Switch: sw, Action: domain.Action(paused), Operator: operator,
		Old: out.Old, New: paused, Changed: out.Changed, At: domain.FormatTime(out.At),
	}, nil
}
