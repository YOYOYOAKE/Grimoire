package preferences

import domainpreferences "grimoire/internal/domain/preferences"

type Repository interface {
	Get() (domainpreferences.Preference, error)
	Save(preference domainpreferences.Preference) error
}
