package infrastructure

import (
	domainaccount "UnpakSiamida/modules/account/domain"
	"crypto/tls"
	"fmt"
	"log"
	"net"
	"os"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/go-ldap/ldap/v3"
)

type LdapRepository struct{}

func NewLdapRepository() *LdapRepository {
	return &LdapRepository{}
}

func (r *LdapRepository) Connect() (*ldap.Conn, error) {
	adServer := os.Getenv("AD_SERVER")
	adUser := os.Getenv("AD_USER")
	adPass := os.Getenv("AD_PASS")

	tlsConfig := &tls.Config{InsecureSkipVerify: true}
	dialer := &net.Dialer{Timeout: 1 * time.Minute}

	var conn *ldap.Conn
	var err error

	if strings.HasPrefix(adServer, "ldaps://") {
		conn, err = ldap.DialURL(adServer, ldap.DialWithTLSConfig(tlsConfig), ldap.DialWithDialer(dialer))
	} else {
		conn, err = ldap.DialURL(adServer, ldap.DialWithDialer(dialer))
		if err == nil {
			_ = conn.StartTLS(tlsConfig)
		}
	}

	if err != nil {
		log.Printf("[LDAP Connect Notice] Server %s unreachable (timeout 3s): %v", adServer, err)
		return nil, fmt.Errorf("failed to connect to LDAP server (%s): %w", adServer, err)
	}

	if adUser != "" && adPass != "" {
		err = conn.Bind(adUser, adPass)
		if err != nil {
			conn.Close()
			log.Printf("[LDAP Bind Notice] User: %s, Error: %v", adUser, err)
			return nil, fmt.Errorf("failed to bind to LDAP: %w", err)
		}
	}

	return conn, nil
}

func (r *LdapRepository) GetAccountsByGroups(allowedGroups []string) ([]domainaccount.AccountLdap, error) {
	if len(allowedGroups) == 0 {
		allowedGroups = []string{
			"adm_simonev_prodi",
			"adm_simonev_fakultas",
			"adm_simonev",
			"adm_pusat",
			"admin",
			"superadmin",
		}
	}

	// Direct Active Directory LDAP Search (ldaps://ldap.unpak.ac.id:636)
	conn, err := r.Connect()
	if err == nil && conn != nil {
		defer conn.Close()

		ldapDN := os.Getenv("LDAP_DN")
		if ldapDN == "" {
			ldapDN = "DC=unpak,DC=ac,DC=id"
		}

		result := []domainaccount.AccountLdap{}
		seen := make(map[string]bool)
		cnRegex := regexp.MustCompile(`(?i)CN=([^,]+)`)

		// Helper to select matched group according to explicit priority order:
		// 1. adm_simonev_prodi
		// 2. adm_simonev_fakultas
		// 3. adm_simonev
		// 4. adm_pusat
		getMatchedGroup := func(userGroups []string) string {
			priorityOrder := []string{
				"adm_simonev_prodi",
				"adm_simonev_fakultas",
				"adm_simonev",
				"adm_pusat",
			}

			var targets []string
			seenTarget := make(map[string]bool)

			for _, p := range priorityOrder {
				targets = append(targets, p)
				seenTarget[strings.ToLower(p)] = true
			}

			for _, grp := range allowedGroups {
				gClean := strings.TrimSpace(grp)
				gLower := strings.ToLower(gClean)
				if gLower != "" && !seenTarget[gLower] {
					targets = append(targets, gClean)
					seenTarget[gLower] = true
				}
			}

			for _, target := range targets {
				targetLower := strings.ToLower(strings.TrimSpace(target))
				for _, ug := range userGroups {
					ugClean := strings.TrimSpace(ug)
					ugLower := strings.ToLower(ugClean)
					if ugLower == targetLower || strings.HasPrefix(ugLower, targetLower) {
						return target
					}
				}
			}

			for _, ug := range userGroups {
				ugLower := strings.ToLower(strings.TrimSpace(ug))
				if strings.Contains(ugLower, "simonev") {
					return ug
				}
			}

			if len(allowedGroups) > 0 {
				return allowedGroups[0]
			}
			return ""
		}

		// Search users directly in Active Directory
		var userFilterParts []string
		for _, grp := range allowedGroups {
			userFilterParts = append(userFilterParts, fmt.Sprintf("(memberOf=CN=%s,OU=Groups,%s)", ldap.EscapeFilter(grp), ldapDN))
			userFilterParts = append(userFilterParts, fmt.Sprintf("(memberOf=CN=%s,%s)", ldap.EscapeFilter(grp), ldapDN))
			userFilterParts = append(userFilterParts, fmt.Sprintf("(memberOf=*CN=%s*)", ldap.EscapeFilter(grp)))
		}
		userFilterParts = append(userFilterParts, "(memberOf=*simonev*)")
		userFilterParts = append(userFilterParts, "(sAMAccountName=*simonev*)")

		userFilter := fmt.Sprintf("(&(objectClass=user)(|%s))", strings.Join(userFilterParts, ""))

		userSearchReq := ldap.NewSearchRequest(
			ldapDN,
			ldap.ScopeWholeSubtree,
			ldap.NeverDerefAliases,
			0, 0, false,
			userFilter,
			[]string{"dn", "sAMAccountName", "displayName", "cn", "mail", "employeeID", "employeeNumber", "memberOf"},
			nil,
		)

		uRes, err := conn.Search(userSearchReq)
		if err == nil && len(uRes.Entries) > 0 {
			for _, uEntry := range uRes.Entries {
				username := uEntry.GetAttributeValue("sAMAccountName")
				if username == "" {
					username = uEntry.GetAttributeValue("cn")
				}
				if username == "" || seen[username] {
					continue
				}
				seen[username] = true

				name := uEntry.GetAttributeValue("displayName")
				if name == "" {
					name = uEntry.GetAttributeValue("cn")
				}
				email := uEntry.GetAttributeValue("mail")
				empID := uEntry.GetAttributeValue("employeeID")
				if empID == "" {
					empID = uEntry.GetAttributeValue("employeeNumber")
				}

				rawMemberOf := uEntry.GetAttributeValues("memberOf")
				var userGroups []string

				for _, m := range rawMemberOf {
					matches := cnRegex.FindStringSubmatch(m)
					if len(matches) > 1 {
						gName := strings.TrimSpace(matches[1])
						if gName != "" {
							userGroups = append(userGroups, gName)
						}
					}
				}

				matchedGroup := getMatchedGroup(userGroups)

				result = append(result, domainaccount.AccountLdap{
					DN:           uEntry.DN,
					Username:     username,
					Name:         name,
					Email:        email,
					EmployeeID:   empID,
					Groups:       userGroups,
					MatchedGroup: matchedGroup,
				})
			}
		}

		// Also search groups in Active Directory
		var groupFilterParts []string
		for _, grp := range allowedGroups {
			groupFilterParts = append(groupFilterParts, fmt.Sprintf("(cn=%s)", ldap.EscapeFilter(grp)))
			groupFilterParts = append(groupFilterParts, fmt.Sprintf("(sAMAccountName=%s)", ldap.EscapeFilter(grp)))
		}
		groupFilterParts = append(groupFilterParts, "(cn=*simonev*)")

		groupFilter := fmt.Sprintf("(&(objectClass=group)(|%s))", strings.Join(groupFilterParts, ""))

		groupSearchReq := ldap.NewSearchRequest(
			ldapDN,
			ldap.ScopeWholeSubtree,
			ldap.NeverDerefAliases,
			0, 0, false,
			groupFilter,
			[]string{"cn", "sAMAccountName", "member"},
			nil,
		)

		groupRes, err := conn.Search(groupSearchReq)
		if err == nil && len(groupRes.Entries) > 0 {
			for _, gEntry := range groupRes.Entries {
				grpCN := gEntry.GetAttributeValue("cn")
				if grpCN == "" {
					grpCN = gEntry.GetAttributeValue("sAMAccountName")
				}

				members := gEntry.GetAttributeValues("member")
				for _, mDN := range members {
					if seen[mDN] || strings.TrimSpace(mDN) == "" {
						continue
					}

					userReq := ldap.NewSearchRequest(
						mDN,
						ldap.ScopeBaseObject,
						ldap.NeverDerefAliases,
						0, 0, false,
						"(objectClass=*)",
						[]string{"dn", "sAMAccountName", "displayName", "cn", "mail", "employeeID", "employeeNumber", "memberOf"},
						nil,
					)

					uRes, err := conn.Search(userReq)
					if err != nil || len(uRes.Entries) == 0 {
						continue
					}

					uEntry := uRes.Entries[0]
					username := uEntry.GetAttributeValue("sAMAccountName")
					if username == "" {
						username = uEntry.GetAttributeValue("cn")
					}
					if username == "" || seen[username] {
						continue
					}
					seen[username] = true
					seen[mDN] = true

					name := uEntry.GetAttributeValue("displayName")
					if name == "" {
						name = uEntry.GetAttributeValue("cn")
					}
					email := uEntry.GetAttributeValue("mail")
					empID := uEntry.GetAttributeValue("employeeID")
					if empID == "" {
						empID = uEntry.GetAttributeValue("employeeNumber")
					}

					rawMemberOf := uEntry.GetAttributeValues("memberOf")
					var userGroups []string

					for _, m := range rawMemberOf {
						matches := cnRegex.FindStringSubmatch(m)
						if len(matches) > 1 {
							gName := strings.TrimSpace(matches[1])
							if gName != "" {
								userGroups = append(userGroups, gName)
							}
						}
					}

					if grpCN != "" {
						hasGrpCN := false
						for _, ug := range userGroups {
							if strings.EqualFold(ug, grpCN) {
								hasGrpCN = true
								break
							}
						}
						if !hasGrpCN {
							userGroups = append(userGroups, grpCN)
						}
					}

					matchedGroup := getMatchedGroup(userGroups)

					result = append(result, domainaccount.AccountLdap{
						DN:           uEntry.DN,
						Username:     username,
						Name:         name,
						Email:        email,
						EmployeeID:   empID,
						Groups:       userGroups,
						MatchedGroup: matchedGroup,
					})
				}
			}
		}

		if len(result) > 0 {
			// Sort results by matched_group priority rank, then by name
			groupRank := func(grp string) int {
				grpLower := strings.ToLower(strings.TrimSpace(grp))
				switch {
				case grpLower == "adm_simonev_prodi" || strings.HasPrefix(grpLower, "adm_simonev_prodi"):
					return 1
				case grpLower == "adm_simonev_fakultas" || strings.HasPrefix(grpLower, "adm_simonev_fakultas"):
					return 2
				case grpLower == "adm_simonev" || strings.HasPrefix(grpLower, "adm_simonev"):
					return 3
				case grpLower == "adm_pusat" || strings.HasPrefix(grpLower, "adm_pusat"):
					return 4
				default:
					return 5
				}
			}

			sort.SliceStable(result, func(i, j int) bool {
				rI := groupRank(result[i].MatchedGroup)
				rJ := groupRank(result[j].MatchedGroup)
				if rI != rJ {
					return rI < rJ
				}
				return strings.ToLower(result[i].Name) < strings.ToLower(result[j].Name)
			})

			log.Printf("[LdapRepository] Successfully fetched %d users from Active Directory LDAP (%s)", len(result), os.Getenv("AD_SERVER"))
			return result, nil
		}
	}

	var fallbackUsers []domainaccount.AccountLdap
	return fallbackUsers, nil
}
