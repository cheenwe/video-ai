package settings

import "strings"

// SecretPlaceholder is returned by GET APIs when a secret is configured.
const SecretPlaceholder = "********"

func IsSecretPlaceholder(v string) bool {
	v = strings.TrimSpace(v)
	return v == "" || v == SecretPlaceholder || v == "***" || v == "****"
}

func RedactEmail(e Email) Email {
	if strings.TrimSpace(e.SMTPPass) != "" {
		e.SMTPPass = SecretPlaceholder
	}
	return e
}

func MergeEmailSecrets(incoming, existing Email) Email {
	if IsSecretPlaceholder(incoming.SMTPPass) {
		incoming.SMTPPass = existing.SMTPPass
	}
	return incoming
}

func RedactLDAP(l LDAP) LDAP {
	if strings.TrimSpace(l.BindPass) != "" {
		l.BindPass = SecretPlaceholder
	}
	return l
}

func MergeLDAPSecrets(incoming, existing LDAP) LDAP {
	if IsSecretPlaceholder(incoming.BindPass) {
		incoming.BindPass = existing.BindPass
	}
	return incoming
}

func RedactOAuth(cfg OAuth) OAuth {
	out := cfg
	out.Providers = make([]OAuthProvider, len(cfg.Providers))
	copy(out.Providers, cfg.Providers)
	for i := range out.Providers {
		if strings.TrimSpace(out.Providers[i].ClientSecret) != "" {
			out.Providers[i].ClientSecret = SecretPlaceholder
		}
	}
	return out
}

func MergeOAuthSecrets(incoming, existing OAuth) OAuth {
	byID := map[string]string{}
	for _, p := range existing.Providers {
		byID[p.ID] = p.ClientSecret
	}
	for i := range incoming.Providers {
		if IsSecretPlaceholder(incoming.Providers[i].ClientSecret) {
			if old, ok := byID[incoming.Providers[i].ID]; ok {
				incoming.Providers[i].ClientSecret = old
			} else {
				incoming.Providers[i].ClientSecret = ""
			}
		}
	}
	return incoming
}

func RedactCloudStorage(cfg CloudStorage) CloudStorage {
	out := cfg
	out.Apps = make([]CloudStorageApp, len(cfg.Apps))
	copy(out.Apps, cfg.Apps)
	for i := range out.Apps {
		if strings.TrimSpace(out.Apps[i].ClientSecret) != "" {
			out.Apps[i].ClientSecret = SecretPlaceholder
		}
	}
	return out
}

func MergeCloudStorageSecrets(incoming, existing CloudStorage) CloudStorage {
	byPlatform := map[string]string{}
	for _, a := range existing.Apps {
		byPlatform[a.Platform] = a.ClientSecret
	}
	for i := range incoming.Apps {
		if IsSecretPlaceholder(incoming.Apps[i].ClientSecret) {
			if old, ok := byPlatform[incoming.Apps[i].Platform]; ok {
				incoming.Apps[i].ClientSecret = old
			} else {
				incoming.Apps[i].ClientSecret = ""
			}
		}
	}
	return incoming
}
