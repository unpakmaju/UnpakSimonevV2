package application

import (
	"context"
	"errors"
	"time"

	domainaccount "UnpakSiamida/modules/account/domain"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

type UpdateAccountCommandHandler struct {
	Repo domainaccount.IAccountRepository
}

func (h *UpdateAccountCommandHandler) Handle(
	ctx context.Context,
	cmd UpdateAccountCommand,
) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	// -------------------------
	// VALIDATE ID
	// -------------------------
	uuidAccount, err := uuid.Parse(cmd.Uuid)
	if err != nil {
		return "", domainaccount.InvalidUuid()
	}

	// -------------------------
	// GET EXISTING ACCOUNT
	// -------------------------
	existingAccount, err := h.Repo.GetByUuid(ctx, uuidAccount)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return "", domainaccount.NotFound(cmd.Uuid)
		}
		return "", err
	}

	// -------------------------
	// DOMAIN LOGIC
	// -------------------------
	result := domainaccount.UpdateAccount(
		existingAccount,
		uuidAccount,
		cmd.Username,
		cmd.Password,
		cmd.Level,
		cmd.Name,
		cmd.Email,
		cmd.EmployeeID,
		cmd.Fakultas,
		cmd.Prodi,
	)

	if !result.IsSuccess {
		return "", result.Error
	}

	updatedAccount := result.Value

	// -------------------------
	// SAVE TO REPOSITORY
	// -------------------------
	if err := h.Repo.Update(ctx, updatedAccount); err != nil {
		return "", err
	}

	return updatedAccount.UUID.String(), nil
}
