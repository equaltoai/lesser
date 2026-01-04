package common

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestDomainErrorTypes_ErrorStrings(t *testing.T) {
	assert.Contains(t, (ActorNotFoundError{Username: "alice"}).Error(), "alice")
	assert.Contains(t, (ActivityNotFoundError{ID: "a1"}).Error(), "a1")
	assert.Contains(t, (ValidationError{Field: "f", Message: "m"}).Error(), "validation failed")
	assert.Contains(t, (AuthenticationError{Message: "nope"}).Error(), "authentication failed")
	assert.Contains(t, (AuthorizationError{Action: "read", Resource: "thing"}).Error(), "not authorized")
	assert.Contains(t, (ConflictError{Resource: "thing", Message: "boom"}).Error(), "conflict")
	assert.Contains(t, (UserNotFoundError{Username: "bob"}).Error(), "bob")
	assert.Contains(t, (AccountSuspendedError{Username: "bob"}).Error(), "bob")
	assert.Equal(t, "invalid password", (InvalidPasswordError{}).Error())
	assert.Contains(t, (InvalidTokenError{Token: "t"}).Error(), "t")
	assert.Contains(t, (ExpiredTokenError{Token: "t"}).Error(), "t")
	assert.Contains(t, (UsedTokenError{Token: "t"}).Error(), "t")
	assert.Contains(t, (SessionNotFoundError{SessionID: "s"}).Error(), "s")
	assert.Contains(t, (AlreadyFollowingError{Follower: "a", Followee: "b"}).Error(), "a")
	assert.Contains(t, (ListNotFoundError{ID: "l"}).Error(), "l")

	cause := errors.New("boom")
	fedErr := FederationError{Operation: "fetch", Remote: "remote", Err: cause}
	assert.Contains(t, fedErr.Error(), "federation fetch failed")
	assert.ErrorIs(t, fedErr.Unwrap(), cause)
}

func TestDomainErrorTypePredicates(t *testing.T) {
	assert.True(t, IsNotFound(ActorNotFoundError{Username: "alice"}))
	assert.True(t, IsNotFound(ActivityNotFoundError{ID: "a1"}))
	assert.False(t, IsNotFound(errors.New("boom")))

	assert.True(t, IsValidation(ValidationError{Field: "f", Message: "m"}))
	assert.False(t, IsValidation(errors.New("boom")))

	assert.True(t, IsAuthentication(AuthenticationError{Message: "nope"}))
	assert.False(t, IsAuthentication(errors.New("boom")))

	assert.True(t, IsAuthorization(AuthorizationError{Action: "a", Resource: "r"}))
	assert.False(t, IsAuthorization(errors.New("boom")))

	assert.True(t, IsConflict(ConflictError{Resource: "r", Message: "m"}))
	assert.False(t, IsConflict(errors.New("boom")))

	assert.True(t, IsFederation(FederationError{Operation: "o", Remote: "r", Err: errors.New("boom")}))
	assert.False(t, IsFederation(errors.New("boom")))
}
