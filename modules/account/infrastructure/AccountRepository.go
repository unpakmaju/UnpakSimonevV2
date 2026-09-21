package infrastructure

import (
	"context"
	"crypto/md5"
	"crypto/sha1"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"gorm.io/gorm"

	commonDomain "UnpakSiamida/common/domain"
	"UnpakSiamida/common/helper"
	domain "UnpakSiamida/modules/account/domain"
)

type AccountRepository struct {
	db       *gorm.DB
	dbSimak  *gorm.DB
	dbSimpeg *gorm.DB
}

func NewAccountRepository(db *gorm.DB, dbsimak *gorm.DB, dbsimpeg *gorm.DB) domain.IAccountRepository {
	return &AccountRepository{
		db:       db,
		dbSimak:  dbsimak,
		dbSimpeg: dbsimpeg,
	}
}

func (r *AccountRepository) Auth(ctx context.Context, username string, password string) (*domain.AccountDefault, error) {

	// Rekursi chain function
	var chain func(func(ctx context.Context) (*domain.AccountDefault, error), ...func(ctx context.Context) (*domain.AccountDefault, error)) (*domain.AccountDefault, error)
	chain = func(current func(ctx context.Context) (*domain.AccountDefault, error), rest ...func(ctx context.Context) (*domain.AccountDefault, error)) (*domain.AccountDefault, error) {
		user, err := current(ctx)
		if err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				// Not found → lanjut ke function berikutnya
				if len(rest) == 0 {
					return nil, gorm.ErrRecordNotFound
				}
				return chain(rest[0], rest[1:]...)
			}
			// Fatal error → stop
			return nil, err
		}

		if user != nil && user.ID != "" {
			return user, nil
		}

		if len(rest) == 0 {
			return nil, gorm.ErrRecordNotFound
		}
		return chain(rest[0], rest[1:]...)
	}

	// Panggil chain sesuai urutan prioritas: authDB -> authSimak -> authSimpeg
	return chain(
		func(ctx context.Context) (*domain.AccountDefault, error) { return r.authDB(ctx, username, password) },
		func(ctx context.Context) (*domain.AccountDefault, error) { return r.authSimak(ctx, username, password) },
		func(ctx context.Context) (*domain.AccountDefault, error) {
			return r.authSimpeg(ctx, username, password)
		},
	)
}

func (r *AccountRepository) Get(ctx context.Context, id domain.AccountIdentifier, mode *string) (*domain.AccountDefault, error) {
	if strings.TrimSpace(helper.StringValue(id.NIDN)) != "" {
		res, err := r.getSimakDosen(ctx, helper.StringValue(id.NIDN))
		if err == nil && res != nil && res.ID != "" {
			if mode == nil {
				local, err := r.getDB(ctx, res.ID)
				if err == nil {
					res.Level = local.Level
					res.Fakultas = local.Fakultas
					res.RefFakultas = local.RefFakultas
					res.Prodi = local.Prodi
					res.RefProdi = local.RefProdi
				}
			}

			return res, nil
		}
	}

	if strings.TrimSpace(helper.StringValue(id.NIM)) != "" {
		res, err := r.getSimakMahasiswa(ctx, helper.StringValue(id.NIM))
		if err == nil && res != nil && res.ID != "" {
			return res, nil
		}
	}

	if strings.TrimSpace(helper.StringValue(id.NIP)) != "" {
		res, err := r.getSimpeg(ctx, id.NIP, id.NIDN)
		if err == nil && res != nil && res.ID != "" {
			if mode == nil {
				local, err := r.getDB(ctx, res.ID)
				if err == nil {
					res.Level = local.Level
					res.Fakultas = local.Fakultas
					res.RefFakultas = local.RefFakultas
					res.Prodi = local.Prodi
					res.RefProdi = local.RefProdi
				}
			}

			return res, nil
		}
	}

	if strings.TrimSpace(helper.StringValue(id.UserID)) != "" {
		sid := helper.StringValue(id.UserID)
		// 1. Try local DB
		res, err := r.getDB(ctx, sid)
		if err == nil && res != nil && res.ID != "" {
			return res, nil
		}
		// 2. Try SIMAK Dosen
		res, err = r.getSimakDosen(ctx, sid)
		if err == nil && res != nil && res.ID != "" {
			return res, nil
		}
		// 3. Try SIMAK Mahasiswa
		res, err = r.getSimakMahasiswa(ctx, sid)
		if err == nil && res != nil && res.ID != "" {
			return res, nil
		}
		// 4. Try SIMPEG Tendik
		res, err = r.getSimpeg(ctx, &sid, nil)
		if err == nil && res != nil && res.ID != "" {
			return res, nil
		}
	}

	identifier := ""
	if id.UserID != nil && strings.TrimSpace(*id.UserID) != "" {
		identifier = strings.TrimSpace(*id.UserID)
	} else if id.NIM != nil && strings.TrimSpace(*id.NIM) != "" {
		identifier = strings.TrimSpace(*id.NIM)
	} else if id.NIDN != nil && strings.TrimSpace(*id.NIDN) != "" {
		identifier = strings.TrimSpace(*id.NIDN)
	} else if id.NIP != nil && strings.TrimSpace(*id.NIP) != "" {
		identifier = strings.TrimSpace(*id.NIP)
	}

	if identifier != "" {
		res := &domain.AccountDefault{
			ID:       identifier,
			Username: helper.StrPtr(identifier),
			Name:     helper.StrPtr(identifier),
			Resource: helper.StrPtr("local"),
		}
		if id.NIM != nil && strings.TrimSpace(*id.NIM) != "" {
			res.Level = helper.StrPtr("mahasiswa")
			res.CodeCtx = helper.StrPtr(domain.CtxMahasiswa)
			res.Resource = helper.StrPtr("simak")
		} else if id.NIDN != nil && strings.TrimSpace(*id.NIDN) != "" {
			res.Level = helper.StrPtr("dosen")
			res.CodeCtx = helper.StrPtr(domain.CtxDosen)
			res.Resource = helper.StrPtr("simak")
		} else if id.NIP != nil && strings.TrimSpace(*id.NIP) != "" {
			res.Level = helper.StrPtr("tendik")
			res.Resource = helper.StrPtr("simpeg")
		} else {
			res.Level = helper.StrPtr("admin")
		}
		return res, nil
	}

	return nil, errors.New("identifier not provided or user not found")
}

func (r *AccountRepository) authDB(ctx context.Context, username string, password string) (*domain.AccountDefault, error) {

	var user domain.AccountDefault

	err := r.db.Debug().WithContext(ctx).
		Table("users").
		Select(`"local" as Resource, null as CodeCtx, users.*`).
		Where("username = ? AND password_plain = ?", username, password).
		First(&user).Error

	if err != nil {
		return nil, err
	}

	return &user, nil
}

func (r *AccountRepository) authSimak(ctx context.Context, username string, password string) (*domain.AccountDefault, error) {

	var user domain.AccountDefault

	query := `
WITH dosen_cte AS (
    SELECT 
        d.nidn,
        CAST(d.nama_dosen AS CHAR(255)) AS Name,
        CAST(NULLIF(TRIM(d.email), '') AS CHAR(255)) AS Email,
        f.kode_fakultas AS RefFakultas,
        f.nama_fakultas AS Fakultas,
        p.kode_prodi AS RefProdi,
        CONCAT(
			p.nama_prodi,
			CASE p.kode_jenjang
				WHEN 'J' THEN ' (Profesi)'
				WHEN 'E' THEN ' (D3)'
				WHEN 'D' THEN ' (D4)'
				WHEN 'A' THEN ' (S3)'
				WHEN 'B' THEN ' (S2)'
				WHEN 'C' THEN ' (S1)'
				ELSE ''
			END
		) AS Prodi 
    FROM m_dosen d
    LEFT JOIN m_fakultas f ON f.kode_fakultas = d.kode_fak
    LEFT JOIN m_program_studi p ON p.kode_prodi = d.kode_prodi
),
mahasiswa_cte AS (
    SELECT
        m.nim,
        CAST(m.nama_mahasiswa AS CHAR(255)) AS Name,
        CAST(NULLIF(TRIM(m.email), '') AS CHAR(255)) AS Email,
        f.kode_fakultas AS RefFakultas,
        f.nama_fakultas AS Fakultas,
        p.kode_prodi AS RefProdi,
        CONCAT(
			p.nama_prodi,
			CASE p.kode_jenjang
				WHEN 'J' THEN ' (Profesi)'
				WHEN 'E' THEN ' (D3)'
				WHEN 'D' THEN ' (D4)'
				WHEN 'A' THEN ' (S3)'
				WHEN 'B' THEN ' (S2)'
				WHEN 'C' THEN ' (S1)'
				ELSE ''
			END
		) AS Prodi 
    FROM m_mahasiswa m
    LEFT JOIN m_fakultas f ON f.kode_fakultas = m.kode_fak
    LEFT JOIN m_program_studi p ON p.kode_prodi = m.kode_prodi
)
SELECT
	u.userid AS ID,
	"simak" AS Resource,
    u.username AS Username,
    u.password AS Password,
    LOWER(u.level) AS Level,
    COALESCE(d.Name, m.Name) AS Name,
    COALESCE(d.Email, m.Email) AS Email,
    COALESCE(d.RefFakultas, m.RefFakultas) AS RefFakultas,
    COALESCE(d.Fakultas, m.Fakultas) AS Fakultas,
    COALESCE(d.RefProdi, m.RefProdi) AS RefProdi,
    COALESCE(d.Prodi, m.Prodi) AS Prodi,
    NULL AS Unit,
	CASE 
		WHEN LENGTH(TRIM(d.Name)) > 0 THEN ?
		WHEN LENGTH(TRIM(m.Name)) > 0 THEN ?
		ELSE ''
	END AS CodeCtx
FROM user u
LEFT JOIN dosen_cte d ON d.nidn = u.userid
LEFT JOIN mahasiswa_cte m ON m.nim = u.userid
WHERE 
	u.username = ?
	AND u.password = ?
	AND u.level IN ("MAHASISWA","DOSEN")
	AND u.aktif = "Y"
LIMIT 1;
`
	hash := sha1.Sum([]byte(md5String(password)))
	hashString := hex.EncodeToString(hash[:])

	err := r.dbSimak.Debug().WithContext(ctx).
		Raw(query, domain.CtxDosen, domain.CtxMahasiswa, username, hashString).
		Scan(&user).Error

	if err != nil {
		return nil, err
	}

	return &user, nil
}

func (r *AccountRepository) authSimpeg(ctx context.Context, username string, password string) (*domain.AccountDefault, error) {

	var user domain.AccountDefault

	query := `
SELECT 
    p.nip as ID,
    'simpeg' as Resource,
    p.nip as Username,
    '' as Password,
    'tendik' as Level,
    TRIM(CONCAT(
        COALESCE(CONCAT(NULLIF(TRIM(p.gelar_depan), ''), ' '), ''),
        p.nama,
        COALESCE(CONCAT(', ', NULLIF(TRIM(p.gelar_belakang), '')), '')
    )) as Name,
    p.email as Email,
    CASE 
        WHEN mu.kode_unit IN ('01','02','03','04','05','06','07','08') THEN mu.kode_unit
        ELSE NULL 
    END as RefFakultas,
    CASE 
        WHEN mu.kode_unit IN ('01','02','03','04','05','06','07','08') THEN mu.nama_unit
        ELSE NULL 
    END as Fakultas,
    NULL as RefProdi,
    NULL as Prodi,
    mu.nama_unit as Unit,
    NULL as CodeCtx
FROM pegawais p
LEFT JOIN (
    SELECT pegawai_id, kode_unit
    FROM pegawai_pekerjaans
    WHERE deleted_at IS NULL
    ORDER BY (status_berlaku = 'Y' OR status_berlaku = 'aktif' OR status_berlaku = '1') DESC, created_at DESC
) pp ON pp.pegawai_id = p.id
LEFT JOIN master_units mu ON mu.kode_unit = pp.kode_unit AND mu.deleted_at IS NULL
WHERE p.deleted_at IS NULL AND (p.nip = ? OR p.nidn_nitk = ? OR p.id = ?)
LIMIT 1
`

	err := r.dbSimpeg.Debug().WithContext(ctx).
		Raw(query, username, username, username).
		Scan(&user).Error

	if err != nil {
		return nil, err
	}

	if user.ID == "" {
		return nil, gorm.ErrRecordNotFound
	}

	return &user, nil
}

func (r *AccountRepository) getDB(ctx context.Context, userid string) (*domain.AccountDefault, error) {

	var user domain.AccountDefault

	err := r.db.WithContext(ctx).
		Table("users u").
		Debug().
		Select(`
			u.*,
			u.employee_id as EmployeeID,
			u.fakultas as RefFakultas,
			f.nama_fakultas as Fakultas,
			u.prodi as RefProdi,
			p.nama_prodi as Prodi,
			null as Unit,
			'local' as Resource,
			null as CodeCtx
		`).
		Joins("LEFT JOIN m_fakultas f ON f.kode_fakultas = u.fakultas").
		Joins("LEFT JOIN m_program_studi p ON p.kode_prodi = u.prodi").
		Where("u.id = ? OR u.username = ? OR u.uuid = ? OR u.employee_id = ?", userid, userid, userid, userid).
		First(&user).Error

	if err != nil {
		return nil, err
	}

	return &user, nil
}

func (r *AccountRepository) getSimakDosen(ctx context.Context, nidn string) (*domain.AccountDefault, error) {
	cleanNidn := strings.TrimSpace(nidn)
	if cleanNidn == "" {
		return nil, gorm.ErrRecordNotFound
	}

	var user domain.AccountDefault

	queryUser := `
WITH dosen_cte AS (
    SELECT 
        d.nidn,
        CAST(d.nama_dosen AS CHAR(255)) AS Name,
        CAST(NULLIF(TRIM(d.email), '') AS CHAR(255)) AS Email,
        f.kode_fakultas AS RefFakultas,
        f.nama_fakultas AS Fakultas,
        p.kode_prodi AS RefProdi,
        CONCAT(
			p.nama_prodi,
			CASE p.kode_jenjang
				WHEN 'J' THEN ' (Profesi)'
				WHEN 'E' THEN ' (D3)'
				WHEN 'D' THEN ' (D4)'
				WHEN 'A' THEN ' (S3)'
				WHEN 'B' THEN ' (S2)'
				WHEN 'C' THEN ' (S1)'
				ELSE ''
			END
		) AS Prodi 
    FROM m_dosen d
    LEFT JOIN m_fakultas f ON f.kode_fakultas = d.kode_fak
    LEFT JOIN m_program_studi p ON p.kode_prodi = d.kode_prodi
)
SELECT
	COALESCE(NULLIF(TRIM(d.nidn), ''), u.userid, u.username, ?) as ID,
	"simak" as Resource,
    COALESCE(u.username, d.nidn, ?) AS Username,
    u.password AS Password,
    'dosen' AS Level,
    COALESCE(NULLIF(TRIM(d.Name), ''), NULLIF(TRIM(u.nama), ''), u.username) AS Name,
    COALESCE(d.Email, u.email) AS Email,
    COALESCE(NULLIF(TRIM(d.nidn), ''), u.userid, ?) AS EmployeeID,
    COALESCE(d.RefFakultas, f.kode_fakultas) AS RefFakultas,
    COALESCE(d.Fakultas, f.nama_fakultas) AS Fakultas,
    COALESCE(d.RefProdi, pr.kode_prodi) AS RefProdi,
    COALESCE(d.Prodi, CONCAT(pr.nama_prodi, CASE pr.kode_jenjang WHEN 'J' THEN ' (Profesi)' WHEN 'E' THEN ' (D3)' WHEN 'D' THEN ' (D4)' WHEN 'A' THEN ' (S3)' WHEN 'B' THEN ' (S2)' WHEN 'C' THEN ' (S1)' ELSE '' END)) AS Prodi,
    COALESCE(f.nama_fakultas, d.Fakultas) AS Unit,
	'` + domain.CtxDosen + `' AS CodeCtx
FROM user u
LEFT JOIN dosen_cte d ON (d.nidn = u.userid OR d.nidn = u.username)
LEFT JOIN m_fakultas f ON f.kode_fakultas = u.fakultas
LEFT JOIN m_program_studi pr ON pr.kode_prodi = u.prodi
WHERE (u.userid = ? OR u.username = ?) AND (u.level = "DOSEN" OR u.level = "dosen")
LIMIT 1
`
	err := r.dbSimak.WithContext(ctx).
		Raw(queryUser, cleanNidn, cleanNidn, cleanNidn, cleanNidn, cleanNidn).
		Scan(&user).Error

	if err == nil && user.ID != "" {
		return &user, nil
	}

	queryDosen := `
SELECT
	COALESCE(NULLIF(TRIM(d.nidn), ''), ?) as ID,
	"simak" as Resource,
    COALESCE(d.nidn, ?) AS Username,
    '' AS Password,
    'dosen' AS Level,
    CAST(d.nama_dosen AS CHAR(255)) AS Name,
    CAST(NULLIF(TRIM(d.email), '') AS CHAR(255)) AS Email,
    COALESCE(NULLIF(TRIM(d.nidn), ''), ?) AS EmployeeID,
    f.kode_fakultas AS RefFakultas,
    f.nama_fakultas AS Fakultas,
    p.kode_prodi AS RefProdi,
    CONCAT(
		p.nama_prodi,
		CASE p.kode_jenjang
			WHEN 'J' THEN ' (Profesi)'
			WHEN 'E' THEN ' (D3)'
			WHEN 'D' THEN ' (D4)'
			WHEN 'A' THEN ' (S3)'
			WHEN 'B' THEN ' (S2)'
			WHEN 'C' THEN ' (S1)'
			ELSE ''
		END
	) AS Prodi,
    f.nama_fakultas AS Unit,
	'` + domain.CtxDosen + `' AS CodeCtx
FROM m_dosen d
LEFT JOIN m_fakultas f ON f.kode_fakultas = d.kode_fak
LEFT JOIN m_program_studi p ON p.kode_prodi = d.kode_prodi
WHERE d.nidn = ?
LIMIT 1
`
	err = r.dbSimak.WithContext(ctx).
		Raw(queryDosen, cleanNidn, cleanNidn, cleanNidn, cleanNidn).
		Scan(&user).Error

	if err == nil && user.ID != "" {
		return &user, nil
	}

	return nil, gorm.ErrRecordNotFound
}

func (r *AccountRepository) getSimakMahasiswa(ctx context.Context, nim string) (*domain.AccountDefault, error) {
	cleanNim := strings.TrimSpace(nim)
	if cleanNim == "" {
		return nil, gorm.ErrRecordNotFound
	}

	var user domain.AccountDefault

	queryUser := `
WITH mahasiswa_cte AS (
    SELECT
        m.nim,
        CAST(m.nama_mahasiswa AS CHAR(255)) AS Name,
        CAST(NULLIF(TRIM(m.email), '') AS CHAR(255)) AS Email,
        f.kode_fakultas AS RefFakultas,
        f.nama_fakultas AS Fakultas,
        p.kode_prodi AS RefProdi,
        CONCAT(
			p.nama_prodi,
			CASE p.kode_jenjang
				WHEN 'J' THEN ' (Profesi)'
				WHEN 'E' THEN ' (D3)'
				WHEN 'D' THEN ' (D4)'
				WHEN 'A' THEN ' (S3)'
				WHEN 'B' THEN ' (S2)'
				WHEN 'C' THEN ' (S1)'
				ELSE ''
			END
		) AS Prodi 
    FROM m_mahasiswa m
    LEFT JOIN m_fakultas f ON f.kode_fakultas = m.kode_fak
    LEFT JOIN m_program_studi p ON p.kode_prodi = m.kode_prodi
)
SELECT
	COALESCE(NULLIF(TRIM(m.nim), ''), u.userid, u.username, ?) as ID,
	"simak" as Resource,
    COALESCE(u.username, m.nim, ?) AS Username,
    u.password AS Password,
    'mahasiswa' AS Level,
    COALESCE(NULLIF(TRIM(m.Name), ''), NULLIF(TRIM(u.nama), ''), u.username) AS Name,
    COALESCE(m.Email, u.email) AS Email,
    COALESCE(NULLIF(TRIM(m.nim), ''), u.userid, ?) AS EmployeeID,
    COALESCE(m.RefFakultas, f.kode_fakultas) AS RefFakultas,
    COALESCE(m.Fakultas, f.nama_fakultas) AS Fakultas,
    COALESCE(m.RefProdi, pr.kode_prodi) AS RefProdi,
    COALESCE(m.Prodi, CONCAT(pr.nama_prodi, CASE pr.kode_jenjang WHEN 'J' THEN ' (Profesi)' WHEN 'E' THEN ' (D3)' WHEN 'D' THEN ' (D4)' WHEN 'A' THEN ' (S3)' WHEN 'B' THEN ' (S2)' WHEN 'C' THEN ' (S1)' ELSE '' END)) AS Prodi,
    COALESCE(f.nama_fakultas, m.Fakultas) AS Unit,
	'` + domain.CtxMahasiswa + `' AS CodeCtx
FROM user u
LEFT JOIN mahasiswa_cte m ON (m.nim = u.userid OR m.nim = u.username)
LEFT JOIN m_fakultas f ON f.kode_fakultas = u.fakultas
LEFT JOIN m_program_studi pr ON pr.kode_prodi = u.prodi
WHERE (u.userid = ? OR u.username = ?) AND (u.level = "MAHASISWA" OR u.level = "mahasiswa")
LIMIT 1
`
	err := r.dbSimak.WithContext(ctx).
		Raw(queryUser, cleanNim, cleanNim, cleanNim, cleanNim, cleanNim).
		Scan(&user).Error

	if err == nil && user.ID != "" {
		return &user, nil
	}

	queryMahasiswa := `
SELECT
	COALESCE(NULLIF(TRIM(m.nim), ''), ?) as ID,
	"simak" as Resource,
    COALESCE(m.nim, ?) AS Username,
    '' AS Password,
    'mahasiswa' AS Level,
    CAST(m.nama_mahasiswa AS CHAR(255)) AS Name,
    CAST(NULLIF(TRIM(m.email), '') AS CHAR(255)) AS Email,
    COALESCE(NULLIF(TRIM(m.nim), ''), ?) AS EmployeeID,
    f.kode_fakultas AS RefFakultas,
    f.nama_fakultas AS Fakultas,
    p.kode_prodi AS RefProdi,
    CONCAT(
		p.nama_prodi,
		CASE p.kode_jenjang
			WHEN 'J' THEN ' (Profesi)'
			WHEN 'E' THEN ' (D3)'
			WHEN 'D' THEN ' (D4)'
			WHEN 'A' THEN ' (S3)'
			WHEN 'B' THEN ' (S2)'
			WHEN 'C' THEN ' (S1)'
			ELSE ''
		END
	) AS Prodi,
    f.nama_fakultas AS Unit,
	'` + domain.CtxMahasiswa + `' AS CodeCtx
FROM m_mahasiswa m
LEFT JOIN m_fakultas f ON f.kode_fakultas = m.kode_fak
LEFT JOIN m_program_studi p ON p.kode_prodi = m.kode_prodi
WHERE m.nim = ?
LIMIT 1
`
	err = r.dbSimak.WithContext(ctx).
		Raw(queryMahasiswa, cleanNim, cleanNim, cleanNim, cleanNim).
		Scan(&user).Error

	if err == nil && user.ID != "" {
		return &user, nil
	}

	return nil, gorm.ErrRecordNotFound
}

func (r *AccountRepository) getSimpeg(ctx context.Context, nip *string, nidn *string) (*domain.AccountDefault, error) {

	var user domain.AccountDefault
	cleanNip := strings.TrimSpace(helper.StringValue(nip))
	cleanNidn := strings.TrimSpace(helper.StringValue(nidn))

	query := `
SELECT 
    p.nip as ID,
    'simpeg' as Resource,
    p.nip as Username,
    p.nip as EmployeeID,
    '' as Password,
    'tendik' as Level,
    TRIM(CONCAT(
        COALESCE(CONCAT(NULLIF(TRIM(p.gelar_depan), ''), ' '), ''),
        p.nama,
        COALESCE(CONCAT(', ', NULLIF(TRIM(p.gelar_belakang), '')), '')
    )) as Name,
    p.email as Email,
    CASE 
        WHEN mu.kode_unit IN ('01','02','03','04','05','06','07','08') THEN mu.kode_unit
        ELSE NULL 
    END as RefFakultas,
    CASE 
        WHEN mu.kode_unit IN ('01','02','03','04','05','06','07','08') THEN mu.nama_unit
        ELSE NULL 
    END as Fakultas,
    NULL as RefProdi,
    NULL as Prodi,
    mu.nama_unit as Unit,
    NULL as CodeCtx
FROM pegawais p
LEFT JOIN (
    SELECT pegawai_id, kode_unit
    FROM pegawai_pekerjaans
    WHERE deleted_at IS NULL
    ORDER BY (status_berlaku = 'Y' OR status_berlaku = 'aktif' OR status_berlaku = '1') DESC, created_at DESC
) pp ON pp.pegawai_id = p.id
LEFT JOIN master_units mu ON mu.kode_unit = pp.kode_unit AND mu.deleted_at IS NULL
WHERE p.deleted_at IS NULL AND (p.nip = ? OR p.nidn_nitk = ? OR p.id = ?)
LIMIT 1
`

	err := r.dbSimpeg.WithContext(ctx).
		Raw(query, cleanNip, cleanNip, cleanNip).
		Scan(&user).Error

	if err != nil {
		return nil, err
	}

	if user.ID == "" && cleanNidn != "" {
		err = r.dbSimpeg.WithContext(ctx).
			Raw(query, cleanNidn, cleanNidn, cleanNidn).
			Scan(&user).Error
		if err != nil {
			return nil, err
		}
	}

	if user.ID == "" {
		return nil, gorm.ErrRecordNotFound
	}

	return &user, nil
}

// ------------------------
// GET BY UUID
// ------------------------
func (r *AccountRepository) GetByUuid(
	ctx context.Context,
	uid uuid.UUID,
) (*domain.Account, error) {

	var account domain.Account

	err := r.db.WithContext(ctx).
		Debug().
		Table("users u").
		Select(`
			u.id as ID,
			u.uuid as UUID,
			u.username as Username,
			u.password_plain as Password,
			u.level as Level,
			u.name as Name,
			u.email as Email,
			u.fakultas as RefFakultas,
			f.nama_fakultas as Fakultas,
			u.prodi as RefProdi,
			p.nama_prodi as Prodi,
			u.deleted_at as DeletedAt,
			u.created_at as CreatedAt,
			u.updated_at as UpdatedAt
		`).
		Joins("LEFT JOIN m_fakultas f ON f.kode_fakultas = u.fakultas").
		Joins("LEFT JOIN m_program_studi p ON p.kode_prodi = u.prodi").
		Where("u.uuid = ?", uid).
		Take(&account).Error

	if err != nil {
		return nil, err
	}

	return &account, nil
}

var allowedSearchColumns = map[string]string{
	"username":      "u.username",
	"name":          "u.name",
	"email":         "u.email",
	"level":         "u.level",
	"fakultas":      "u.fakultas",
	"prodi":         "u.prodi",
	"nama_fakultas": "f.nama_fakultas",
	"nama_prodi": `
		CONCAT(
			p.nama_prodi,
			CASE p.kode_jenjang
				WHEN 'J' THEN ' (Profesi)'
				WHEN 'E' THEN ' (D3)'
				WHEN 'D' THEN ' (D4)'
				WHEN 'A' THEN ' (S3)'
				WHEN 'B' THEN ' (S2)'
				WHEN 'C' THEN ' (S1)'
				ELSE ''
			END
		)
	`,
}

// ------------------------
// GET ALL
// ------------------------
func (r *AccountRepository) GetAll(
	ctx context.Context,
	search string,
	searchFilters []commonDomain.SearchFilter,
	page, limit *int,
	deleted bool,
) ([]domain.Account, int64, error) {

	var rows []domain.Account
	var total int64

	db := r.db.WithContext(ctx).
		Table("users u").
		Select(`
			u.id as ID,
			u.uuid as UUID,
			u.username as Username,
			u.level as Level,
			u.name as Name,
			u.email as Email,
			u.fakultas as RefFakultas,
			f.nama_fakultas as Fakultas,
			u.prodi as RefProdi,
			CONCAT(
				p.nama_prodi,
				CASE p.kode_jenjang
					WHEN 'J' THEN ' (Profesi)'
					WHEN 'E' THEN ' (D3)'
					WHEN 'D' THEN ' (D4)'
					WHEN 'A' THEN ' (S3)'
					WHEN 'B' THEN ' (S2)'
					WHEN 'C' THEN ' (S1)'
					ELSE ''
				END
			) AS Prodi, 
			u.deleted_at as DeletedAt,
			u.created_at as CreatedAt,
			u.updated_at as UpdatedAt
		`).
		Joins("LEFT JOIN m_fakultas f ON f.kode_fakultas = u.fakultas").
		Joins("LEFT JOIN m_program_studi p ON p.kode_prodi = u.prodi")

	if deleted {
		db = db.Where("u.deleted_at IS NOT NULL")
	} else {
		db = db.Where("u.deleted_at IS NULL")
	}

	// ------------------------
	// ADVANCED FILTER
	// ------------------------
	for _, f := range searchFilters {
		col, ok := allowedSearchColumns[strings.ToLower(f.Field)]
		if !ok || f.Value == nil {
			continue
		}

		val := strings.TrimSpace(*f.Value)
		if val == "" {
			continue
		}

		switch strings.ToLower(f.Operator) {
		case "eq":
			db = db.Where(fmt.Sprintf("%s = ?", col), val)

		case "neq":
			db = db.Where(fmt.Sprintf("%s != ?", col), val)

		case "like":
			db = db.Where(
				fmt.Sprintf("%s LIKE ?", col),
				"%"+helper.EscapeLike(val)+"%",
			)

		case "in":
			rawVals := strings.Split(val, ",")
			vals := make([]interface{}, 0)

			for _, v := range rawVals {
				v = strings.TrimSpace(v)
				if v != "" {
					vals = append(vals, v)
				}
			}

			if len(vals) > 0 {
				db = db.Where(fmt.Sprintf("%s IN ?", col), vals)
			}
		}
	}

	// ------------------------
	// GLOBAL SEARCH
	// ------------------------
	if s := strings.TrimSpace(search); s != "" {
		like := "%" + helper.EscapeLike(s) + "%"

		var conditions []string
		var args []interface{}

		for _, col := range allowedSearchColumns {
			conditions = append(
				conditions,
				fmt.Sprintf("%s LIKE ?", col),
			)
			args = append(args, like)
		}

		db = db.Where(
			"("+strings.Join(conditions, " OR ")+")",
			args...,
		)
	}

	// ------------------------
	// COUNT
	// ------------------------
	if err := db.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	// ------------------------
	// PAGINATION
	// ------------------------
	db = db.Order("u.id DESC")

	if page != nil && limit != nil && *limit > 0 {
		offset := (*page - 1) * (*limit)
		db = db.Offset(offset).Limit(*limit)
	}

	// ------------------------
	// EXECUTE
	// ------------------------
	if err := db.Find(&rows).Error; err != nil {
		return nil, 0, err
	}

	return rows, total, nil
}

// ------------------------
// CREATE
// ------------------------
func (r *AccountRepository) Create(
	ctx context.Context,
	account *domain.Account,
) error {
	return r.db.WithContext(ctx).
		Create(account).Error
}

// ------------------------
// UPDATE
// ------------------------
func (r *AccountRepository) Update(
	ctx context.Context,
	account *domain.Account,
) error {
	return r.db.WithContext(ctx).Debug().
		Save(account).Error
}

// ------------------------
// DELETE
// ------------------------
func (r *AccountRepository) Delete(
	ctx context.Context,
	uid uuid.UUID,
) error {
	return r.db.WithContext(ctx).
		Where("uuid = ?", uid).
		Delete(&domain.Account{}).Error
}

// ------------------------
// SETUP UUID
// ------------------------
func (r *AccountRepository) SetupUuid(
	ctx context.Context,
) error {

	const chunkSize = 500

	var ids []string

	if err := r.db.WithContext(ctx).
		Model(&domain.Account{}).
		Where("uuid IS NULL OR uuid = ''").
		Pluck("id", &ids).Error; err != nil {
		return err
	}

	if len(ids) == 0 {
		return nil
	}

	for i := 0; i < len(ids); i += chunkSize {
		end := i + chunkSize
		if end > len(ids) {
			end = len(ids)
		}

		chunk := ids[i:end]

		caseSQL := "CASE id "
		args := make([]any, 0)

		for _, id := range chunk {
			caseSQL += "WHEN ? THEN ? "
			args = append(args, id, uuid.NewString())
		}

		caseSQL += "END"

		args = append(args, chunk)

		query := fmt.Sprintf(`
			UPDATE users
			SET uuid = %s
			WHERE id IN (?)
		`, caseSQL)

		if err := r.db.WithContext(ctx).
			Exec(query, args...).Error; err != nil {
			return err
		}
	}

	return nil
}

func md5String(text string) string {
	hash := md5.Sum([]byte(text))
	return hex.EncodeToString(hash[:])
}
