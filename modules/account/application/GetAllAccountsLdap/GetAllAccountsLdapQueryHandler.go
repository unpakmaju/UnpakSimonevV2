package application

import (
	domainAccount "UnpakSiamida/modules/account/domain"
	"context"
	"time"
)

type ILdapRepository interface {
	GetAccountsByGroups(allowedGroups []string) ([]domainAccount.AccountLdap, error)
}

type GetAllAccountsLdapQueryHandler struct {
	LdapRepo ILdapRepository
}

func (h *GetAllAccountsLdapQueryHandler) Handle(
	ctx context.Context,
	q GetAllAccountsLdapQuery,
) ([]domainAccount.AccountLdap, error) {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	targetGroups := q.Groups
	if len(targetGroups) == 0 {
		targetGroups = []string{
			"adm_simonev_prodi",
			"adm_simonev_fakultas",
			"adm_simonev",
		}
	}

	return h.LdapRepo.GetAccountsByGroups(targetGroups)
}
