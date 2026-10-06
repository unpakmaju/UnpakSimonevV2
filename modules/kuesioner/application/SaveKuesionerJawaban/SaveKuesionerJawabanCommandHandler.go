package application

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	helper "UnpakSiamida/common/helper"
	domainkuesioner "UnpakSiamida/modules/kuesioner/domain"
	domainjawaban "UnpakSiamida/modules/templatejawaban/domain"
	domainpertanyaan "UnpakSiamida/modules/templatepertanyaan/domain"

	"github.com/google/uuid"
)

type JawabanPayload struct {
	UUID     string `json:"uuid"`
	FreeText string `json:"freetext"`
}

type SaveKuesionerJawabanCommandHandler struct {
	Repo                 domainkuesioner.IKuesionerRepository
	RepoPertanyaan       domainpertanyaan.ITemplatePertanyaanRepository
	RepoJawaban          domainjawaban.ITemplateJawabanRepository
	RepoJawabanKuesioner domainkuesioner.IKuesionerJawabanRepository
}

func (h *SaveKuesionerJawabanCommandHandler) Handle(
	ctx context.Context,
	cmd SaveKuesionerJawabanCommand,
) (string, error) {

	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	// ===============================
	// VALIDASI UUID
	// ===============================
	kuesionerUUID, err := uuid.Parse(cmd.UuidKuesioner)
	if err != nil {
		return "", domainkuesioner.InvalidUuid()
	}

	pertanyaanUUID, err := uuid.Parse(cmd.UuidPertanyaan)
	if err != nil {
		return "", domainkuesioner.InvalidPertanyaan()
	}

	kuesioner, err := h.Repo.GetByUuid(ctx, kuesionerUUID)
	if err != nil {
		return "", err
	}

	pertanyaan, err := h.RepoPertanyaan.GetByUuid(ctx, pertanyaanUUID)
	if err != nil {
		return "", err
	}

	// ===============================
	// PARSE JSON
	// ===============================
	var raw []JawabanPayload
	if err := json.Unmarshal([]byte(cmd.Jawaban), &raw); err != nil {
		return "", domainkuesioner.InvalidJawaban()
	}

	var selectedUUIDs []string
	var freeTexts []string
	freeTextMap := map[string]string{}

	for _, item := range raw {

		if item.UUID != "" {
			if _, err := uuid.Parse(item.UUID); err != nil {
				return "", errors.New("uuid jawaban tidak valid")
			}
			selectedUUIDs = append(selectedUUIDs, item.UUID)
		}

		if item.FreeText != "" {
			freeTexts = append(freeTexts, item.FreeText)

			if item.UUID != "" {
				freeTextMap[item.UUID] = item.FreeText
			}
		}
	}

	// ===============================
	// MAP UUID → ID
	// ===============================
	jawabanList, err := h.RepoJawaban.GetByUUIDs(ctx, selectedUUIDs)
	if err != nil {
		return "", err
	}

	selectedMap := map[uint]bool{}
	for _, j := range jawabanList {
		selectedMap[j.ID] = true
	}

	// ===============================
	// FREE TEXT TEMPLATE
	// ===============================
	freeTemplate, err := h.RepoJawaban.GetFreeTextByPertanyaan(ctx, pertanyaan.ID)
	if err != nil {
		return "", err
	}

	// ===============================
	// TRANSACTION
	// ===============================
	tx, err := h.RepoJawabanKuesioner.BeginTx(ctx)
	if err != nil {
		return "", err
	}

	defer func() {
		if r := recover(); r != nil {
			_ = tx.Rollback()
			panic(r)
		}
	}()

	commit := false
	defer func() {
		if !commit {
			_ = tx.Rollback()
		}
	}()

	// ===============================
	// EXISTING DATA
	// ===============================
	existing, err := h.RepoJawabanKuesioner.
		WithTx(tx).
		GetByPertanyaanAndUser(ctx, kuesioner.ID, pertanyaan.ID, cmd.SID, cmd.Resource)

	if err != nil {
		return "", err
	}

	existingMap := map[uint]domainkuesioner.KuesionerJawaban{}
	var existingFree []domainkuesioner.KuesionerJawaban

	for _, v := range existing {
		if v.IdTemplateJawaban != nil {
			if freeTemplate != nil && *v.IdTemplateJawaban == freeTemplate.ID {
				existingFree = append(existingFree, v)
			} else {
				existingMap[*v.IdTemplateJawaban] = v
			}
		}
	}

	// ===============================
	// DELETE
	// ===============================
	for id, data := range existingMap {
		if !selectedMap[id] {
			if err := h.RepoJawabanKuesioner.WithTx(tx).Delete(ctx, data.ID); err != nil {
				return "", err
			}
		}
	}

	// ===============================
	// INSERT JAWABAN PILIHAN
	// ===============================
	for _, j := range jawabanList {
		if _, exist := existingMap[j.ID]; !exist {

			var freeText *string
			if val, ok := freeTextMap[j.UUID.String()]; ok {
				freeText = &val
			}

			newData := domainkuesioner.KuesionerJawaban{
				UUID:                 helper.StrPtr(uuid.New().String()),
				IdKuesioner:          kuesioner.ID,
				IdTemplatePertanyaan: pertanyaan.ID,
				IdTemplateJawaban:    &j.ID,
				FreeText:             freeText,
				CreatedBy:            &cmd.Resource,
				CreatedByRef:         &cmd.SID,
			}

			if err := h.RepoJawabanKuesioner.WithTx(tx).Create(ctx, &newData); err != nil {
				return "", err
			}
		}
	}

	// ===============================
	// FREE TEXT VALIDATION
	// ===============================
	if len(freeTexts) > 0 && freeTemplate == nil {
		return "", errors.New("pertanyaan ini tidak mendukung free text")
	}

	existingCount := len(existingFree)
	inputCount := len(freeTexts)

	min := existingCount
	if inputCount < min {
		min = inputCount
	}

	// update existing
	for i := 0; i < min; i++ {
		text := freeTexts[i]
		existingFree[i].FreeText = &text

		if err := h.RepoJawabanKuesioner.WithTx(tx).Create(ctx, &existingFree[i]); err != nil {
			return "", err
		}
	}

	// insert new
	if inputCount > existingCount {
		for i := existingCount; i < inputCount; i++ {

			text := freeTexts[i]

			newData := domainkuesioner.KuesionerJawaban{
				UUID:                 helper.StrPtr(uuid.New().String()),
				IdKuesioner:          kuesioner.ID,
				IdTemplatePertanyaan: pertanyaan.ID,
				IdTemplateJawaban:    &freeTemplate.ID,
				FreeText:             &text,
				CreatedBy:            &cmd.Resource,
				CreatedByRef:         &cmd.SID,
			}

			if err := h.RepoJawabanKuesioner.WithTx(tx).Create(ctx, &newData); err != nil {
				return "", err
			}
		}
	}

	// delete excess
	if existingCount > inputCount {
		for i := inputCount; i < existingCount; i++ {
			if err := h.RepoJawabanKuesioner.WithTx(tx).Delete(ctx, existingFree[i].ID); err != nil {
				return "", err
			}
		}
	}

	// ===============================
	// COMMIT
	// ===============================
	if err := tx.Commit().Error; err != nil {
		return "", err
	}

	commit = true

	return kuesioner.UUID.String(), nil
}
