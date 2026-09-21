package application

import (
	"context"
	"errors"
	"testing"

	"UnpakSiamida/common/helper"
	mockrepo "UnpakSiamida/modules/account/application/mock"
	domainaccount "UnpakSiamida/modules/account/domain"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"gorm.io/gorm"
)

func TestUpdateAccountCommandHandler_Handle(t *testing.T) {
	validUUID := uuid.New()

	t.Run("Success", func(t *testing.T) {
		existingAccount := &domainaccount.Account{
			UUID:     validUUID,
			Password: helper.StrPtr("oldpassword"),
		}

		repo := &mockrepo.MockAccountRepository{
			GetByUuidFunc: func(ctx context.Context, uid uuid.UUID) (*domainaccount.Account, error) {
				assert.Equal(t, validUUID, uid)
				return existingAccount, nil
			},
			UpdateFunc: func(ctx context.Context, account *domainaccount.Account) error {
				assert.Equal(t, "updated-user", *account.Username)
				assert.Equal(t, "newpassword", *account.Password)
				return nil
			},
		}

		handler := &UpdateAccountCommandHandler{
			Repo: repo,
		}

		cmd := UpdateAccountCommand{
			Uuid:     validUUID.String(),
			Username: "updated-user",
			Password: helper.StrPtr("newpassword"),
			Level:    "admin",
			Name:     "Updated Name",
			Email:    helper.StrPtr("updated@test.com"),
		}

		res, err := handler.Handle(context.Background(), cmd)
		assert.NoError(t, err)
		assert.Equal(t, validUUID.String(), res)
	})

	t.Run("Failure_InvalidUuid", func(t *testing.T) {
		handler := &UpdateAccountCommandHandler{
			Repo: &mockrepo.MockAccountRepository{},
		}

		cmd := UpdateAccountCommand{
			Uuid: "invalid-uuid",
		}

		res, err := handler.Handle(context.Background(), cmd)
		assert.Error(t, err)
		assert.Equal(t, domainaccount.InvalidUuid().Error(), err.Error())
		assert.Empty(t, res)
	})

	t.Run("Failure_NotFound", func(t *testing.T) {
		repo := &mockrepo.MockAccountRepository{
			GetByUuidFunc: func(ctx context.Context, uid uuid.UUID) (*domainaccount.Account, error) {
				return nil, gorm.ErrRecordNotFound
			},
		}

		handler := &UpdateAccountCommandHandler{
			Repo: repo,
		}

		cmd := UpdateAccountCommand{
			Uuid: validUUID.String(),
		}

		res, err := handler.Handle(context.Background(), cmd)
		assert.Error(t, err)
		assert.Equal(t, domainaccount.NotFound(validUUID.String()).Error(), err.Error())
		assert.Empty(t, res)
	})

	t.Run("Failure_GetByUuidError", func(t *testing.T) {
		expectedErr := errors.New("db read error")
		repo := &mockrepo.MockAccountRepository{
			GetByUuidFunc: func(ctx context.Context, uid uuid.UUID) (*domainaccount.Account, error) {
				return nil, expectedErr
			},
		}

		handler := &UpdateAccountCommandHandler{
			Repo: repo,
		}

		cmd := UpdateAccountCommand{
			Uuid: validUUID.String(),
		}

		res, err := handler.Handle(context.Background(), cmd)
		assert.ErrorIs(t, err, expectedErr)
		assert.Empty(t, res)
	})

	t.Run("Failure_DomainInvalidData", func(t *testing.T) {
		// Repo returns an account with mismatching UUID
		existingAccount := &domainaccount.Account{
			UUID: uuid.New(), // different UUID
		}

		repo := &mockrepo.MockAccountRepository{
			GetByUuidFunc: func(ctx context.Context, uid uuid.UUID) (*domainaccount.Account, error) {
				return existingAccount, nil
			},
		}

		handler := &UpdateAccountCommandHandler{
			Repo: repo,
		}

		cmd := UpdateAccountCommand{
			Uuid: validUUID.String(),
		}

		res, err := handler.Handle(context.Background(), cmd)
		assert.Error(t, err)
		assert.Equal(t, domainaccount.InvalidData().Error(), err.Error())
		assert.Empty(t, res)
	})

	t.Run("Failure_UpdateRepoError", func(t *testing.T) {
		existingAccount := &domainaccount.Account{
			UUID: validUUID,
		}
		expectedErr := errors.New("db update error")
		repo := &mockrepo.MockAccountRepository{
			GetByUuidFunc: func(ctx context.Context, uid uuid.UUID) (*domainaccount.Account, error) {
				return existingAccount, nil
			},
			UpdateFunc: func(ctx context.Context, account *domainaccount.Account) error {
				return expectedErr
			},
		}

		handler := &UpdateAccountCommandHandler{
			Repo: repo,
		}

		cmd := UpdateAccountCommand{
			Uuid:     validUUID.String(),
			Username: "updated-user",
		}

		res, err := handler.Handle(context.Background(), cmd)
		assert.ErrorIs(t, err, expectedErr)
		assert.Empty(t, res)
	})
}
