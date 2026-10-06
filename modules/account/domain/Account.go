package domain

import (
	common "UnpakSiamida/common/domain"
	"UnpakSiamida/common/helper"
	"time"

	"github.com/google/uuid"
)

type Account struct {
	ID          uint       `gorm:"primaryKey;autoIncrement" json:"-"`
	UUID        uuid.UUID  `json:"UUID"`
	Username    *string    `json:"Username"`
	Password    *string    `gorm:"column:password_plain" json:"-"`
	Level       *string    `json:"Level"`
	Name        *string    `json:"Name"`
	Email       *string    `json:"Email"`
	EmployeeID  *string    `gorm:"column:employee_id" json:"EmployeeID"`
	RefFakultas *string    `gorm:"column:fakultas" json:"RefFakultas"`
	Fakultas    *string    `gorm:"column:Fakultas;->" json:"Fakultas"`
	RefProdi    *string    `gorm:"column:prodi" json:"RefProdi"`
	Prodi       *string    `gorm:"column:Prodi;->" json:"Prodi"`
	DeletedAt   *time.Time `gorm:"column:deleted_at"`
	CreatedAt   *time.Time `gorm:"column:created_at"`
	UpdatedAt   *time.Time `gorm:"column:updated_at"`
}

func (Account) TableName() string {
	return "users"
}

// CREATE
// [pr] perlu validasi total
func NewAccount(
	username string,
	password string,
	level string,
	name string,
	email *string,
	employeeID *string,
	refFakultas *string,
	refProdi *string,
) common.ResultValue[*Account] {

	entity := &Account{
		UUID:        uuid.New(),
		Username:    helper.StrPtr(username),
		Password:    helper.StrPtr(password),
		Level:       helper.StrPtr(level),
		Name:        helper.StrPtr(name),
		Email:       email,
		EmployeeID:  employeeID,
		RefFakultas: refFakultas,
		RefProdi:    refProdi,
	}

	return common.SuccessValue(entity)
}

// UPDATE
func UpdateAccount(
	prev *Account,
	uid uuid.UUID,
	username string,
	password *string,
	level string,
	name string,
	email *string,
	employeeID *string,
	refFakultas *string,
	refProdi *string,
) common.ResultValue[*Account] {

	if prev == nil {
		return common.FailureValue[*Account](EmptyData())
	}
	if prev.UUID != uid {
		return common.FailureValue[*Account](InvalidData())
	}

	prev.Username = helper.StrPtr(username)
	prev.Level = helper.StrPtr(level)
	prev.Name = helper.StrPtr(name)
	prev.Email = email
	if employeeID != nil && *employeeID != "" {
		prev.EmployeeID = employeeID
	}
	prev.RefFakultas = refFakultas
	prev.RefProdi = refProdi

	// password optional saat update
	if password != nil && *password != "" {
		prev.Password = password
	}

	return common.SuccessValue(prev)
}

// === Delete ===
func DeleteAccount(
	prev *Account,
) common.ResultValue[*Account] {

	if prev == nil {
		return common.FailureValue[*Account](EmptyData())
	}

	now := time.Now()
	prev.DeletedAt = &now

	return common.SuccessValue(prev)
}

// === Restore ===
func RestoreAccount(
	prev *Account,
) common.ResultValue[*Account] {

	if prev == nil {
		return common.FailureValue[*Account](EmptyData())
	}

	prev.DeletedAt = nil

	return common.SuccessValue(prev)
}
