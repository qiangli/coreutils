package newgrpcmd

import (
	"os"
	"strconv"
)

var currentCredentials = defaultCurrentCredentials

func defaultCurrentCredentials() (credentialState, error) {
	groups, err := os.Getgroups()
	if err != nil {
		return credentialState{}, err
	}
	state := credentialState{
		RealGID:          strconv.Itoa(os.Getgid()),
		EffectiveGID:     strconv.Itoa(os.Getegid()),
		MaxSupplementary: maximumSupplementaryGroups(),
	}
	for _, g := range groups {
		state.Supplementary = append(state.Supplementary, strconv.Itoa(g))
	}
	return state, nil
}
