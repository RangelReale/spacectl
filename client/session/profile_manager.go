package session

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const (
	// SpaceliftConfigDirectory is the name of the Spacelift config directory.
	SpaceliftConfigDirectory = ".spacelift"

	// CurrentFileName is the name of the symlink that points at the current profile.
	CurrentFileName = "current"
)

// A Profile represents a spacectl profile which is used to store credential information
// for accessing Spacelift.
type Profile struct {
	// The name of the profile.
	Alias string `json:"alias,omitempty"`

	// The credentials used to make Spacelift API requests.
	Credentials *StoredCredentials `json:"credentials,omitempty"`
}

// A ProfileManager is used to interact with Spacelift profiles.
type ProfileManager struct {
	// The directory that profiles are stored in.
	ProfilesDirectory string

	// The path to the currently selected profile.
	CurrentPath string
}

// NewProfileManager creates a new ProfileManager using the specified directory to store the profile data.
func NewProfileManager(profilesDirectory string) *ProfileManager {
	return &ProfileManager{
		ProfilesDirectory: profilesDirectory,
		CurrentPath:       filepath.Join(profilesDirectory, CurrentFileName),
	}
}

// Init initialises the profile manager, making sure it is ready to store and retrieve profiles.
func (m *ProfileManager) Init() error {
	return os.MkdirAll(m.ProfilesDirectory, 0700)
}

// Get returns the profile with the specified name.
func (m *ProfileManager) Get(profileName string) (*Profile, error) {
	if profileName == "" {
		return nil, errors.New("a profile name must be specified")
	}

	if _, err := os.Stat(m.ProfilePath(profileName)); err != nil {
		return nil, fmt.Errorf("a profile named '%s' could not be found", profileName)
	}

	return getProfileFromPath(m.ProfilePath(profileName))
}

// Current gets the user's currently selected profile, and returns nil if no profile is selected.
func (m *ProfileManager) Current() (*Profile, error) {
	if _, err := os.Lstat(m.CurrentPath); os.IsNotExist(err) {
		return nil, nil
	}

	return getProfileFromPath(m.CurrentPath)
}

// Select sets the currently selected profile.
func (m *ProfileManager) Select(profileName string) error {
	if _, err := os.Stat(m.ProfilePath(profileName)); err != nil {
		return fmt.Errorf("could not find a profile named '%s'", profileName)
	}

	if _, err := os.Lstat(m.CurrentPath); err == nil {
		if err := os.Remove(m.CurrentPath); err != nil {
			return fmt.Errorf("failed to unlink current config file: %v", err)
		}
	}

	if err := os.Symlink(m.ProfilePath(profileName), m.CurrentPath); err != nil {
		return fmt.Errorf("could not symlink the config file for %s: %w", profileName, err)
	}

	return nil
}

// Create adds a new Spacelift profile.
func (m *ProfileManager) Create(profile *Profile) error {
	if err := validateProfile(profile); err != nil {
		return err
	}

	if err := writeProfileToFile(profile, m); err != nil {
		return err
	}

	setCurrentProfile(m.ProfilePath(profile.Alias), m)

	return nil
}

// Delete removes the profile with the specified name, and un-selects it as the current profile
// if it was selected.
func (m *ProfileManager) Delete(profileName string) error {
	if profileName == "" {
		return errors.New("a profile name must be specified")
	}

	if _, err := os.Stat(m.ProfilePath(profileName)); err != nil {
		return fmt.Errorf("no profile named '%s' exists", profileName)
	}

	if err := os.Remove(m.ProfilePath(profileName)); err != nil {
		return err
	}

	currentTarget, err := os.Readlink(m.CurrentPath)

	switch {
	case os.IsNotExist(err):
		return nil
	case err == nil && currentTarget == m.ProfilePath(profileName):
		return os.Remove(m.CurrentPath)
	default:
		return err
	}
}

// ProfilePath returns the path to the profile with the specified name.
func (m *ProfileManager) ProfilePath(profileName string) string {
	return filepath.Join(m.ProfilesDirectory, profileName)
}

func validateProfile(profile *Profile) error {
	if profile == nil {
		return errors.New("profile must not be nil")
	}

	if profile.Alias == "" {
		return errors.New("a profile name must be specified")
	}

	if strings.Contains(profile.Alias, "/") || strings.Contains(profile.Alias, "\\") || profile.Alias == "current" {
		return fmt.Errorf("'%s' is not a valid profile name", profile.Alias)
	}

	switch credentialType := profile.Credentials.Type; credentialType {
	case CredentialsTypeGitHubToken:
		if err := validateGitHubCredentials(profile); err != nil {
			return err
		}

	case CredentialsTypeAPIKey:
		if err := validateAPIKeyCredentials(profile); err != nil {
			return err
		}

	default:
		return fmt.Errorf("'%d' is an invalid credential type", credentialType)
	}

	return nil
}

func validateGitHubCredentials(profile *Profile) error {
	if profile.Credentials.AccessToken == "" {
		return errors.New("'AccessToken' must be provided for GitHub token credentials")
	}

	return nil
}

func validateAPIKeyCredentials(profile *Profile) error {
	if profile.Credentials.KeyID == "" {
		return errors.New("'KeyID' must be provided for API Key credentials")
	}

	if profile.Credentials.KeySecret == "" {
		return errors.New("'KeySecret' must be provided for API Key credentials")
	}

	return nil
}

func setCurrentProfile(profilePath string, manager *ProfileManager) error {
	if _, err := os.Lstat(manager.CurrentPath); err == nil {
		if err := os.Remove(manager.CurrentPath); err != nil {
			return fmt.Errorf("failed to unlink current config file: %v", err)
		}
	}

	if err := os.Symlink(profilePath, manager.CurrentPath); err != nil {
		return fmt.Errorf("could not symlink the config file for %s: %w", profilePath, err)
	}

	return nil
}

func getProfileFromPath(profilePath string) (*Profile, error) {
	data, err := os.ReadFile(profilePath)
	if err != nil {
		return nil, fmt.Errorf("could not read Spacelift profile from %s: %w", profilePath, err)
	}

	var profile Profile
	if err := json.Unmarshal(data, &profile); err != nil {
		return nil, fmt.Errorf("could not unmarshal Spacelift profile from %s: %w", profilePath, err)
	}

	return &profile, nil
}

func writeProfileToFile(profile *Profile, manager *ProfileManager) error {
	file, err := os.OpenFile(manager.ProfilePath(profile.Alias), os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0600)
	if err != nil {
		return fmt.Errorf("could not create config file for %s: %w", manager.ProfilePath(profile.Alias), err)
	}

	encoder := json.NewEncoder(file)
	encoder.SetIndent("", "  ")

	if err := encoder.Encode(profile); err != nil {
		return fmt.Errorf("could not write config file for %s: %w", manager.ProfilePath(profile.Alias), err)
	}

	if err := file.Close(); err != nil {
		return fmt.Errorf("could close the config file for %s: %w", manager.ProfilePath(profile.Alias), err)
	}

	return nil
}