package domain

import (
	"context"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

type IKuesionerJawabanRepository interface {
	GetByKuesionerAndUser(
		ctx context.Context,
		idKuesioner uint,
		sid string,
		resource string,
	) ([]KuesionerJawaban, error)
	GetByPertanyaanAndUser(
		ctx context.Context,
		idKuesioner uint,
		pertanyaanID uint,
		sid string,
		resource string,
	) ([]KuesionerJawaban, error)
	GetTotalInputByKuesionerIDs(
		ctx context.Context,
		ids []uint,
		targetUUIDs ...[]uuid.UUID,
	) (map[string]uint, error)
	GetAllByKuesioner(
		ctx context.Context,
		uuidkuesioner string,
	) ([]KuesionerJawabanDefault, error)

	Create(ctx context.Context, data *KuesionerJawaban) error
	Delete(ctx context.Context, id uint) error

	WithTx(tx any) IKuesionerJawabanRepository
	BeginTx(ctx context.Context) (*gorm.DB, error)
}
