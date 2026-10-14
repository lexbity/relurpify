package testhelper

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"codeburg.org/lexbit/relurpify/governance/authorization"
	"codeburg.org/lexbit/relurpify/named/euclo/interaction"
	ucperms "codeburg.org/lexbit/relurpify/userconfig/permissions"
)

func hitlRequest(action string) authorization.PermissionRequest {
	return authorization.PermissionRequest{
		Permission:    ucperms.PermissionDescriptor{Action: action},
		Justification: "test " + action,
	}
}

func TestAutoApprovingBrokerGrantsEveryRequest(t *testing.T) {
	broker := NewAutoApprovingBroker()
	ctx := context.Background()

	first, err := broker.RequestPermission(ctx, hitlRequest("fs:write"))
	require.NoError(t, err)
	require.NotNil(t, first)
	assert.Equal(t, "auto-approving-fs:write", first.ID)
	assert.Equal(t, "testhelper", first.ApprovedBy)
	assert.Equal(t, "test fs:write", first.Description)
	assert.False(t, first.ExpiresAt.Before(time.Now().UTC()), "grant must carry a future expiry")

	second, err := broker.RequestPermission(ctx, hitlRequest("net:connect"))
	require.NoError(t, err)
	require.NotNil(t, second)

	requests := broker.Requests()
	require.Len(t, requests, 2)
	assert.Equal(t, "fs:write", requests[0].Permission.Action)
	assert.Equal(t, "net:connect", requests[1].Permission.Action)

	// Requests returns a copy: mutating it must not affect the broker.
	requests[0].Justification = "mutated"
	assert.Equal(t, "test fs:write", broker.Requests()[0].Justification)
}

func TestAutoApprovingBrokerNilReceiver(t *testing.T) {
	var broker *AutoApprovingBroker
	_, err := broker.RequestPermission(context.Background(), hitlRequest("x"))
	require.Error(t, err)
	assert.Nil(t, broker.Requests())
}

func TestScriptedBrokerApprovesScriptedActions(t *testing.T) {
	broker := NewScriptedBroker().Approve("fs:write").Approve("net:connect")
	ctx := context.Background()

	grant, err := broker.RequestPermission(ctx, hitlRequest("fs:write"))
	require.NoError(t, err)
	require.NotNil(t, grant)
	assert.Equal(t, "scripted-fs:write", grant.ID)
	assert.Equal(t, "testhelper", grant.ApprovedBy)

	_, err = broker.RequestPermission(ctx, hitlRequest("net:connect"))
	require.NoError(t, err)
	require.Len(t, broker.Requests(), 2)
}

func TestScriptedBrokerDeniesScriptedActions(t *testing.T) {
	broker := NewScriptedBroker().Deny("shell:exec", "not today")

	grant, err := broker.RequestPermission(context.Background(), hitlRequest("shell:exec"))
	require.Error(t, err)
	assert.Nil(t, grant)
	assert.Contains(t, err.Error(), "scripted denial: not today")
}

func TestScriptedBrokerFailsClosedWithoutScript(t *testing.T) {
	broker := NewScriptedBroker()

	grant, err := broker.RequestPermission(context.Background(), hitlRequest("unknown:action"))
	require.Error(t, err)
	assert.Nil(t, grant)
	assert.Contains(t, err.Error(), `no script for "unknown:action" (fail-closed)`)
}

func TestScriptedBrokerDefaultAnswer(t *testing.T) {
	broker := NewScriptedBroker()
	broker.Default = &authorization.PermissionGrant{ID: "default-grant", ApprovedBy: "testhelper"}

	grant, err := broker.RequestPermission(context.Background(), hitlRequest("anything"))
	require.NoError(t, err)
	require.NotNil(t, grant)
	assert.Equal(t, "default-grant", grant.ID)
}

func TestScriptedBrokerNilReceiver(t *testing.T) {
	var broker *ScriptedBroker
	assert.Same(t, broker, broker.Approve("x"))
	assert.Same(t, broker, broker.Deny("x", "y"))
	_, err := broker.RequestPermission(context.Background(), hitlRequest("x"))
	require.Error(t, err)
	assert.Nil(t, broker.Requests())
}

func TestPermissiveResolverAnswersDefaultChoice(t *testing.T) {
	resolver := NewPermissiveResolver()
	ctx := context.Background()

	frame := &interaction.InteractionFrame{ID: "frame-1", DefaultChoice: " allow "}
	resolution, err := resolver.Resolve(ctx, frame)
	require.NoError(t, err)
	assert.Equal(t, interaction.ResolutionAnswered, resolution.Status)
	assert.Equal(t, "allow", resolution.Answer)

	// A scripted per-frame resolution wins over the default choice.
	resolver.resolved["frame-1"] = interaction.FrameResolution{Status: interaction.ResolutionAnswered, Answer: "custom"}
	resolution, err = resolver.Resolve(ctx, frame)
	require.NoError(t, err)
	assert.Equal(t, "custom", resolution.Answer)

	// A frame without a default choice resolves as abort.
	resolution, err = resolver.Resolve(ctx, &interaction.InteractionFrame{ID: "frame-2"})
	require.NoError(t, err)
	assert.Equal(t, "abort", resolution.Answer)
}

func TestPermissiveResolverExpiredForMissingFrameIdentity(t *testing.T) {
	resolver := NewPermissiveResolver()

	resolution, err := resolver.Resolve(context.Background(), nil)
	require.NoError(t, err)
	assert.Equal(t, interaction.ResolutionExpired, resolution.Status)

	resolution, err = resolver.Resolve(context.Background(), &interaction.InteractionFrame{ID: "  "})
	require.NoError(t, err)
	assert.Equal(t, interaction.ResolutionExpired, resolution.Status)
}

func TestPermissiveResolverNotifyRecordsFrameIDs(t *testing.T) {
	resolver := NewPermissiveResolver()
	require.NoError(t, resolver.Notify(context.Background(), &interaction.InteractionFrame{ID: "b-frame"}))
	require.NoError(t, resolver.Notify(context.Background(), &interaction.InteractionFrame{ID: "a-frame"}))
	// Nil frames are ignored.
	require.NoError(t, resolver.Notify(context.Background(), nil))

	assert.Equal(t, []string{"a-frame", "b-frame"}, resolver.Notified())
}

func TestPermissiveResolverNilReceiver(t *testing.T) {
	var resolver *PermissiveResolver
	resolution, err := resolver.Resolve(context.Background(), &interaction.InteractionFrame{ID: "x"})
	require.NoError(t, err)
	assert.Equal(t, interaction.ResolutionExpired, resolution.Status)
	assert.NoError(t, resolver.Notify(context.Background(), nil))
	assert.Nil(t, resolver.Notified())
}

func TestScriptedResolverAnswersScriptedFrames(t *testing.T) {
	resolver := NewScriptedResolver().
		Answer("frame-yes", interaction.FrameResolution{Status: interaction.ResolutionAnswered, Answer: "yes"})

	resolution, err := resolver.Resolve(context.Background(), &interaction.InteractionFrame{ID: "frame-yes"})
	require.NoError(t, err)
	assert.Equal(t, "yes", resolution.Answer)

	// Unknown frames expire.
	resolution, err = resolver.Resolve(context.Background(), &interaction.InteractionFrame{ID: "frame-other"})
	require.NoError(t, err)
	assert.Equal(t, interaction.ResolutionExpired, resolution.Status)

	// Answer with an empty status is ignored, leaving the frame unresolved.
	resolver.Answer("frame-yes", interaction.FrameResolution{})
	resolution, err = resolver.Resolve(context.Background(), &interaction.InteractionFrame{ID: "frame-yes"})
	require.NoError(t, err)
	assert.Equal(t, "yes", resolution.Answer)
}

func TestScriptedResolverNotifyAndNilReceiver(t *testing.T) {
	resolver := NewScriptedResolver()
	require.NoError(t, resolver.Notify(context.Background(), &interaction.InteractionFrame{ID: "f1"}))
	require.NoError(t, resolver.Notify(context.Background(), nil))

	var nilResolver *ScriptedResolver
	resolution, err := nilResolver.Resolve(context.Background(), &interaction.InteractionFrame{ID: "f1"})
	require.NoError(t, err)
	assert.Equal(t, interaction.ResolutionExpired, resolution.Status)
	assert.NoError(t, nilResolver.Notify(context.Background(), nil))
}

func TestExpiredClockAdvancesAndServesDeadlineHelper(t *testing.T) {
	start := time.Date(2026, 10, 13, 8, 0, 0, 0, time.UTC)
	clock := NewExpiredClock(start)

	assert.Equal(t, start, clock.Now())
	clock.Advance(90 * time.Second)
	assert.Equal(t, start.Add(90*time.Second), clock.Now())

	deadline := clock.DeadlineHelper()
	assert.Equal(t, clock.Now(), deadline())
}

func TestExpiredClockZeroStartsAtReferenceTime(t *testing.T) {
	clock := NewExpiredClock(time.Time{})
	expected := time.Date(2026, 10, 9, 12, 0, 0, 0, time.UTC)
	assert.Equal(t, expected, clock.Now())
}

func TestExpiredClockNilReceiver(t *testing.T) {
	var clock *ExpiredClock
	assert.False(t, clock.Now().IsZero(), "nil clock must fall back to the real clock")
	assert.NotPanics(t, func() { clock.Advance(time.Minute) })
}

func TestBrokerAndResolverDoublesAreConcurrencySafe(t *testing.T) {
	broker := NewAutoApprovingBroker()
	resolver := NewPermissiveResolver()
	var wg sync.WaitGroup
	for i := 0; i < 16; i++ {
		wg.Add(2)
		go func() { defer wg.Done(); _, _ = broker.RequestPermission(context.Background(), hitlRequest("a")) }()
		go func() {
			defer wg.Done()
			_, _ = resolver.Resolve(context.Background(), &interaction.InteractionFrame{ID: "f"})
		}()
	}
	wg.Wait()
	assert.Len(t, broker.Requests(), 16)
	assert.Empty(t, resolver.Notified(), "Resolve must not imply Notify")
}
