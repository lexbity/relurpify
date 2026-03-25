package main

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/gorilla/websocket"
	nexusadmin "github.com/lexcodex/relurpify/app/nexus/admin"
	nexusbootstrap "github.com/lexcodex/relurpify/app/nexus/bootstrap"
	nexuscfg "github.com/lexcodex/relurpify/app/nexus/config"
	nexusdb "github.com/lexcodex/relurpify/app/nexus/db"
	"github.com/lexcodex/relurpify/app/nexus/gateway"
	nexusserver "github.com/lexcodex/relurpify/app/nexus/server"
	nexusstatus "github.com/lexcodex/relurpify/app/nexus/status"
	"github.com/lexcodex/relurpify/framework/config"
	"github.com/lexcodex/relurpify/framework/core"
	"github.com/lexcodex/relurpify/framework/event"
	"github.com/lexcodex/relurpify/framework/identity"
	memdb "github.com/lexcodex/relurpify/framework/memory/db"
	"github.com/lexcodex/relurpify/framework/middleware/channel"
	fwfmp "github.com/lexcodex/relurpify/framework/middleware/fmp"
	fwgateway "github.com/lexcodex/relurpify/framework/middleware/gateway"
	mcpprotocol "github.com/lexcodex/relurpify/framework/middleware/mcp/protocol"
	mcpserver "github.com/lexcodex/relurpify/framework/middleware/mcp/server"
	fwnode "github.com/lexcodex/relurpify/framework/middleware/node"
	"github.com/spf13/cobra"
)

const gatewayLogCompactionInterval = time.Hour
const nexusEventPartition = "local"

type compactingEventLog interface {
	CompactBefore(ctx context.Context, cutoff time.Time) (int64, error)
}

func main() {
	if err := newRootCmd().Execute(); err != nil {
		log.Fatal(err)
	}
}

func newRootCmd() *cobra.Command {
	var workspace string
	var configPath string
	root := &cobra.Command{
		Use:   "nexus",
		Short: "Nexus gateway entrypoint",
	}
	root.PersistentFlags().StringVar(&workspace, "workspace", ".", "workspace directory")
	root.PersistentFlags().StringVar(&configPath, "config", "", "path to nexus config")
	root.AddCommand(
		newStartCmd(&workspace, &configPath),
		newStatusCmd(&workspace, &configPath),
		newNodeCmd(&workspace, &configPath),
		newAdminCmd(&workspace, &configPath),
	)
	return root
}

func newStartCmd(workspace, configPath *string) *cobra.Command {
	return &cobra.Command{
		Use:   "start",
		Short: "Start the Nexus gateway",
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx, cancel := signal.NotifyContext(cmd.Context(), os.Interrupt, syscall.SIGTERM)
			defer cancel()
			return runStart(ctx, *workspace, *configPath)
		},
	}
}

func newStatusCmd(workspace, configPath *string) *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: "Show Nexus gateway status from local state",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runStatus(cmd.Context(), cmd.OutOrStdout(), *workspace, *configPath)
		},
	}
}

func resolveConfig(workspace, configPath string) (config.Paths, nexuscfg.Config, error) {
	return nexusbootstrap.ResolveConfig(workspace, configPath)
}

func runStart(ctx context.Context, workspace, configPath string) error {
	paths, cfg, err := resolveConfig(workspace, configPath)
	if err != nil {
		return err
	}
	eventLog, err := nexusdb.NewSQLiteEventLog(paths.EventsFile())
	if err != nil {
		return err
	}
	defer eventLog.Close()
	if err := startEventLogCompactor(ctx, eventLog, cfg.Gateway.Log.RetentionDays, gatewayLogCompactionInterval, time.Now); err != nil {
		return err
	}
	stateMaterializer := gateway.NewStateMaterializer()
	materializerRunner := &event.Runner{
		Log:              eventLog,
		Materializers:    []event.Materializer{stateMaterializer},
		Partition:        nexusEventPartition,
		SnapshotInterval: cfg.Gateway.Log.SnapshotInterval,
	}
	if err := materializerRunner.RestoreAndRunOnce(ctx); err != nil {
		return err
	}

	sessionStore, err := nexusdb.NewSQLiteSessionStore(paths.SessionStoreFile())
	if err != nil {
		return err
	}
	defer sessionStore.Close()
	identityStore, err := nexusdb.NewSQLiteIdentityStore(paths.IdentityStoreFile())
	if err != nil {
		return err
	}
	defer identityStore.Close()
	nodeStore, err := nexusdb.NewSQLiteNodeStore(paths.NodesFile())
	if err != nil {
		return err
	}
	defer nodeStore.Close()
	tokenStore, err := nexusdb.NewSQLiteAdminTokenStore(paths.AdminTokenStoreFile())
	if err != nil {
		return err
	}
	defer tokenStore.Close()
	policyStore, err := memdb.NewFilePolicyRuleStore(paths.PolicyRulesFile())
	if err != nil {
		return err
	}
	ownershipStore, err := fwfmp.NewSQLiteOwnershipStore(filepath.Join(paths.ConfigRoot(), "fmp_ownership.db"))
	if err != nil {
		return err
	}
	defer ownershipStore.Close()
	exportStore, err := nexusdb.NewSQLiteFMPExportStore(filepath.Join(paths.ConfigRoot(), "fmp_exports.db"))
	if err != nil {
		return err
	}
	defer exportStore.Close()
	federationStore, err := nexusdb.NewSQLiteFMPFederationStore(filepath.Join(paths.ConfigRoot(), "fmp_federation.db"))
	if err != nil {
		return err
	}
	defer federationStore.Close()
	trustStore, err := nexusdb.NewSQLiteTrustBundleStore(filepath.Join(paths.ConfigRoot(), "fmp_trust_bundles.db"))
	if err != nil {
		return err
	}
	defer trustStore.Close()
	boundaryStore, err := nexusdb.NewSQLiteBoundaryPolicyStore(filepath.Join(paths.ConfigRoot(), "fmp_boundary_policies.db"))
	if err != nil {
		return err
	}
	defer boundaryStore.Close()
	operationalLimiter, err := nexusdb.NewSQLiteOperationalLimiter(filepath.Join(paths.ConfigRoot(), "fmp_operational_limits.db"), fwfmp.OperationalLimits{
		Window:                time.Minute,
		MaxActiveResumeSlots:  8,
		MaxResumeBytesWindow:  32 << 20,
		MaxForwardBytesWindow: 64 << 20,
		MaxFederatedForwards:  256,
	})
	if err != nil {
		return err
	}
	defer operationalLimiter.Close()
	transportNonceStore, err := nexusdb.NewSQLiteTransportNonceStore(filepath.Join(paths.ConfigRoot(), "gateway_transport_nonces.db"))
	if err != nil {
		return err
	}
	defer transportNonceStore.Close()
	transportPolicy := fwgateway.DefaultFMPTransportPolicy(nexuscfg.IsLoopbackBind(cfg.Gateway.Bind))
	transportPolicy.NonceStore = transportNonceStore
	fmpSigner, err := loadOrCreateFMPSigner(filepath.Join(paths.ConfigRoot(), "fmp_signing_seed"))
	if err != nil {
		return err
	}
	fmpVerifier := &fwfmp.Ed25519Verifier{PublicKey: fmpSigner.PublicKey()}
	auditStore, err := nexusdb.NewSQLiteAuditChainStore(filepath.Join(paths.ConfigRoot(), "fmp_audit_chain.db"), fmpSigner, fmpVerifier)
	if err != nil {
		return err
	}
	defer auditStore.Close()
	fmpService := &fwfmp.Service{
		Ownership:  ownershipStore,
		Discovery:  &fwfmp.InMemoryDiscoveryStore{},
		Trust:      trustStore,
		Boundaries: boundaryStore,
		Projector:  fwfmp.StrictCapabilityProjector{},
		Limiter:    operationalLimiter,
		Log:        eventLog,
		Partition:  nexusEventPartition,
		LeaseTTL:   5 * time.Minute,
		Audit:      auditStore,
		Signer:     fmpSigner,
		Nexus: fwfmp.NexusAdapter{
			Exports:    exportStore,
			Federation: federationStore,
			Policies: &fwfmp.AuthorizationPolicyResolver{
				Rules: policyStore,
				TTL:   30 * time.Second,
			},
		},
	}
	handler, err := (&nexusserver.NexusApp{
		EventLog:           eventLog,
		SessionStore:       sessionStore,
		IdentityStore:      identityStore,
		NodeStore:          nodeStore,
		TokenStore:         tokenStore,
		PolicyStore:        policyStore,
		FMPService:         fmpService,
		FMPExportStore:     exportStore,
		FMPFederationStore: federationStore,
		Config:             cfg,
		Partition:          nexusEventPartition,
		Workspace:          workspace,
		StateMaterializer:  stateMaterializer,
		FMPTransportPolicy: transportPolicy,
		StartedAt:          time.Now().UTC(),
		PrincipalResolver:  gatewayPrincipalResolver(cfg.Gateway.Auth, tokenStore, identityStore),
		VerifyNodeConnection: func(ctx context.Context, store identity.Store, principal fwgateway.ConnectionPrincipal, info fwgateway.NodeConnectInfo, conn *websocket.Conn) error {
			return verifyGatewayNodeChallenge(ctx, store, principal, info, conn)
		},
	}).Handler(ctx)
	if err != nil {
		return err
	}

	httpServer := &http.Server{Addr: cfg.Gateway.Bind, Handler: handler}
	go func() {
		<-ctx.Done()
		_ = httpServer.Shutdown(context.Background())
	}()
	if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		return err
	}
	return nil
}

func loadOrCreateFMPSigner(path string) (*fwfmp.Ed25519Signer, error) {
	path = filepath.Clean(path)
	seedText, err := os.ReadFile(path)
	switch {
	case err == nil:
		seed, err := base64.RawStdEncoding.DecodeString(strings.TrimSpace(string(seedText)))
		if err != nil {
			return nil, err
		}
		return fwfmp.NewEd25519SignerFromSeed(seed), nil
	case os.IsNotExist(err):
		seed := make([]byte, 32)
		if _, err := rand.Read(seed); err != nil {
			return nil, err
		}
		if err := os.WriteFile(path, []byte(base64.RawStdEncoding.EncodeToString(seed)), 0o600); err != nil {
			return nil, err
		}
		return fwfmp.NewEd25519SignerFromSeed(seed), nil
	default:
		return nil, err
	}
}

func gatewayPrincipalResolver(cfg nexuscfg.GatewayAuthConfig, tokenStore nexusadmin.TokenStore, identityStore identity.Store) func(context.Context, string) (fwgateway.ConnectionPrincipal, error) {
	if !cfg.Enabled {
		return nil
	}
	// Pre-hash every static token at startup so lookups are O(1) map reads
	// rather than an O(N) constant-time scan.  SHA-256 collision resistance
	// provides the same security guarantee as comparing the raw tokens.
	staticByHash := make(map[string]fwgateway.ConnectionPrincipal, len(cfg.Tokens))
	for _, entry := range cfg.Tokens {
		if entry.Token == "" || entry.SubjectID == "" || entry.Role == "" {
			continue
		}
		tenantID := nexusserver.NormalizeTenantID(entry.TenantID)
		subjectKind := core.SubjectKind(entry.SubjectKind)
		if subjectKind == "" {
			switch strings.ToLower(strings.TrimSpace(entry.Role)) {
			case "node":
				subjectKind = core.SubjectKindNode
			case "agent", "operator", "admin":
				subjectKind = core.SubjectKindServiceAccount
			default:
				subjectKind = core.SubjectKindUser
			}
		}
		principal := core.AuthenticatedPrincipal{
			TenantID:      tenantID,
			AuthMethod:    core.AuthMethodBearerToken,
			Authenticated: true,
			Scopes:        append([]string(nil), entry.Scopes...),
			Subject: core.SubjectRef{
				TenantID: tenantID,
				Kind:     subjectKind,
				ID:       entry.SubjectID,
			},
		}
		staticByHash[nexusadmin.HashToken(entry.Token)] = fwgateway.ConnectionPrincipal{
			Role:          entry.Role,
			Authenticated: true,
			Principal:     &principal,
			Actor: core.EventActor{
				Kind:        entry.Role,
				ID:          entry.SubjectID,
				TenantID:    tenantID,
				SubjectKind: subjectKind,
			},
		}
	}
	return func(ctx context.Context, token string) (fwgateway.ConnectionPrincipal, error) {
		if token == "" {
			return fwgateway.ConnectionPrincipal{}, fmt.Errorf("bearer token required")
		}
		if principal, ok := staticByHash[nexusadmin.HashToken(token)]; ok {
			return principal, nil
		}
		if tokenStore != nil {
			records, err := tokenStore.ListTokens(ctx)
			if err != nil {
				return fwgateway.ConnectionPrincipal{}, fmt.Errorf("lookup bearer token: %w", err)
			}
			hashed := nexusadmin.HashToken(token)
			for _, record := range records {
				if len(record.TokenHash) != len(hashed) {
					continue
				}
				if record.RevokedAt != nil {
					continue
				}
				if record.ExpiresAt != nil && record.ExpiresAt.Before(time.Now().UTC()) {
					continue
				}
				if subtle.ConstantTimeCompare([]byte(record.TokenHash), []byte(hashed)) != 1 {
					continue
				}
				principal, err := dynamicTokenPrincipal(ctx, record, identityStore)
				if err != nil {
					return fwgateway.ConnectionPrincipal{}, err
				}
				return fwgateway.ConnectionPrincipal{
					Role:          principalRole(principal.Scopes),
					Authenticated: true,
					Principal:     &principal,
					Actor: core.EventActor{
						Kind:        principalRole(principal.Scopes),
						ID:          principal.Subject.ID,
						TenantID:    principal.TenantID,
						SubjectKind: principal.Subject.Kind,
					},
				}, nil
			}
		}
		return fwgateway.ConnectionPrincipal{}, fmt.Errorf("unknown bearer token")
	}
}

func staticGatewayPrincipalResolver(cfg nexuscfg.GatewayAuthConfig) func(context.Context, string) (fwgateway.ConnectionPrincipal, error) {
	return gatewayPrincipalResolver(cfg, nil, nil)
}

func dynamicTokenPrincipal(ctx context.Context, record core.AdminTokenRecord, identityStore identity.Store) (core.AuthenticatedPrincipal, error) {
	tenantID := nexusserver.NormalizeTenantID(record.TenantID)
	subjectID := strings.TrimSpace(record.SubjectID)
	if subjectID == "" {
		return core.AuthenticatedPrincipal{}, fmt.Errorf("token %s missing subject id", record.ID)
	}
	subjectKind := record.SubjectKind
	if identityStore != nil {
		tenant, err := identityStore.GetTenant(ctx, tenantID)
		if err != nil {
			return core.AuthenticatedPrincipal{}, fmt.Errorf("lookup token tenant: %w", err)
		}
		if tenant == nil {
			return core.AuthenticatedPrincipal{}, fmt.Errorf("token tenant %s not found", tenantID)
		}
		if tenant.DisabledAt != nil {
			return core.AuthenticatedPrincipal{}, fmt.Errorf("token tenant %s disabled", tenantID)
		}
		if subjectKind != "" {
			subject, err := identityStore.GetSubject(ctx, tenantID, subjectKind, subjectID)
			if err != nil {
				return core.AuthenticatedPrincipal{}, fmt.Errorf("lookup token subject: %w", err)
			}
			if subject == nil {
				return core.AuthenticatedPrincipal{}, fmt.Errorf("token subject %s/%s not found", subjectKind, subjectID)
			}
			if subject.DisabledAt != nil {
				return core.AuthenticatedPrincipal{}, fmt.Errorf("token subject %s/%s disabled", subjectKind, subjectID)
			}
		} else {
			subjects, err := identityStore.ListSubjects(ctx, tenantID)
			if err != nil {
				return core.AuthenticatedPrincipal{}, fmt.Errorf("list token subjects: %w", err)
			}
			for _, subject := range subjects {
				if strings.EqualFold(subject.ID, subjectID) {
					if subject.DisabledAt != nil {
						return core.AuthenticatedPrincipal{}, fmt.Errorf("token subject %s/%s disabled", subject.Kind, subjectID)
					}
					subjectKind = subject.Kind
					break
				}
			}
			if subjectKind == "" {
				return core.AuthenticatedPrincipal{}, fmt.Errorf("token subject %s not found", subjectID)
			}
		}
	}
	if subjectKind == "" {
		subjectKind = core.SubjectKindServiceAccount
	}
	principal := core.AuthenticatedPrincipal{
		TenantID:      tenantID,
		AuthMethod:    core.AuthMethodBearerToken,
		Authenticated: true,
		Scopes:        append([]string(nil), record.Scopes...),
		Subject: core.SubjectRef{
			TenantID: tenantID,
			Kind:     subjectKind,
			ID:       subjectID,
		},
	}
	if err := principal.Validate(); err != nil {
		return core.AuthenticatedPrincipal{}, fmt.Errorf("token principal invalid: %w", err)
	}
	return principal, nil
}

func principalRole(scopes []string) string {
	role := "agent"
	for _, scope := range scopes {
		switch strings.ToLower(strings.TrimSpace(scope)) {
		case "gateway:admin", "nexus:admin", "admin":
			return "admin"
		case "nexus:operator", "operator":
			role = "operator"
		case "node":
			if role == "agent" {
				role = "node"
			}
		}
	}
	return role
}

func runStatus(ctx context.Context, out interface{ Write([]byte) (int, error) }, workspace, configPath string) error {
	snapshot, err := nexusstatus.Load(ctx, workspace, configPath)
	if err != nil {
		return err
	}
	_, err = fmt.Fprint(out, snapshot.Summary())
	return err
}

func startEventLogCompactor(ctx context.Context, logStore compactingEventLog, retentionDays int, interval time.Duration, now func() time.Time) error {
	if logStore == nil || retentionDays <= 0 {
		return nil
	}
	if interval <= 0 {
		interval = gatewayLogCompactionInterval
	}
	if now == nil {
		now = time.Now
	}
	if _, err := compactEventLog(ctx, logStore, retentionDays, now); err != nil {
		return err
	}
	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				if _, err := compactEventLog(ctx, logStore, retentionDays, now); err != nil && ctx.Err() == nil {
					log.Printf("nexus: event log compaction failed: %v", err)
				}
			}
		}
	}()
	return nil
}

func compactEventLog(ctx context.Context, logStore compactingEventLog, retentionDays int, now func() time.Time) (int64, error) {
	if logStore == nil || retentionDays <= 0 {
		return 0, nil
	}
	if now == nil {
		now = time.Now
	}
	cutoff := now().UTC().AddDate(0, 0, -retentionDays)
	return logStore.CompactBefore(ctx, cutoff)
}

func newNodeCmd(workspace, configPath *string) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "node",
		Short: "Manage Nexus node pairing",
	}
	cmd.AddCommand(newNodePairCmd(workspace, configPath), newNodeApproveCmd(workspace, configPath), newNodeRejectCmd(workspace, configPath))
	return cmd
}

func newAdminCmd(workspace, configPath *string) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "admin",
		Short: "Serve Nexus admin APIs",
	}
	cmd.AddCommand(newAdminMCPCmd(workspace, configPath))
	return cmd
}

func newAdminMCPCmd(workspace, configPath *string) *cobra.Command {
	var token string
	cmd := &cobra.Command{
		Use:   "mcp",
		Short: "Serve admin MCP over stdio",
		RunE: func(cmd *cobra.Command, args []string) error {
			paths, cfg, err := resolveConfig(*workspace, *configPath)
			if err != nil {
				return err
			}
			eventLog, err := nexusdb.NewSQLiteEventLog(paths.EventsFile())
			if err != nil {
				return err
			}
			defer eventLog.Close()
			sessionStore, err := nexusdb.NewSQLiteSessionStore(paths.SessionStoreFile())
			if err != nil {
				return err
			}
			defer sessionStore.Close()
			identityStore, err := nexusdb.NewSQLiteIdentityStore(paths.IdentityStoreFile())
			if err != nil {
				return err
			}
			defer identityStore.Close()
			nodeStore, err := nexusdb.NewSQLiteNodeStore(paths.NodesFile())
			if err != nil {
				return err
			}
			defer nodeStore.Close()
			tokenStore, err := nexusdb.NewSQLiteAdminTokenStore(paths.AdminTokenStoreFile())
			if err != nil {
				return err
			}
			defer tokenStore.Close()
			policyStore, err := memdb.NewFilePolicyRuleStore(paths.PolicyRulesFile())
			if err != nil {
				return err
			}
			nodeManager := &fwnode.Manager{
				Store: nodeStore,
				Log:   eventLog,
				Pairing: fwnode.PairingConfig{
					AutoApproveLocal: cfg.Nodes.AutoApproveLocal,
					PairingCodeTTL:   cfg.Nodes.PairingCodeTTL,
				},
			}
			stateMaterializer := gateway.NewStateMaterializer()
			runner := &event.Runner{
				Log:           eventLog,
				Materializers: []event.Materializer{stateMaterializer},
				Partition:     nexusEventPartition,
			}
			if err := runner.RestoreAndRunOnce(cmd.Context()); err != nil {
				return err
			}
			adminSvc := nexusadmin.NewService(nexusadmin.ServiceConfig{
				Nodes:        nodeStore,
				NodeManager:  nodeManager,
				Sessions:     sessionStore,
				Identities:   identityStore,
				Tokens:       tokenStore,
				Policies:     policyStore,
				Events:       eventLog,
				Materializer: stateMaterializer,
				Channels:     channel.NewManager(eventLog, nil),
				Partition:    nexusEventPartition,
				Config:       cfg,
				StartedAt:    time.Now().UTC(),
			})
			adminMCPSvc := mcpserver.New(
				mcpprotocol.PeerInfo{Name: "nexus-admin", Version: nexusadmin.APIVersionV1Alpha1},
				nexusadmin.NewMCPExporter(adminSvc),
				mcpserver.Hooks{},
			)
			principal, err := stdioAdminPrincipal(cfg, tokenStore, identityStore, token)
			if err != nil {
				return err
			}
			return adminMCPSvc.ServeConn(
				nexusadmin.WithPrincipal(cmd.Context(), principal),
				"stdio",
				stdioReadWriteCloser{Reader: os.Stdin, Writer: os.Stdout},
			)
		},
	}
	cmd.Flags().StringVar(&token, "token", "", "admin or operator bearer token for the stdio session")
	return cmd
}

type stdioReadWriteCloser struct {
	io.Reader
	io.Writer
}

func (stdioReadWriteCloser) Close() error { return nil }

func stdioAdminPrincipal(cfg nexuscfg.Config, tokenStore nexusadmin.TokenStore, identityStore identity.Store, token string) (core.AuthenticatedPrincipal, error) {
	if strings.TrimSpace(token) != "" {
		resolver := gatewayPrincipalResolver(cfg.Gateway.Auth, tokenStore, identityStore)
		if resolver == nil {
			return core.AuthenticatedPrincipal{}, fmt.Errorf("gateway auth disabled")
		}
		principal, err := resolver(context.Background(), token)
		if err != nil || principal.Principal == nil {
			return core.AuthenticatedPrincipal{}, fmt.Errorf("resolve admin principal: %w", err)
		}
		return *principal.Principal, nil
	}
	for _, entry := range cfg.Gateway.Auth.Tokens {
		role := strings.ToLower(strings.TrimSpace(entry.Role))
		if role != "admin" && role != "operator" {
			continue
		}
		principal, err := stdioAdminPrincipal(cfg, tokenStore, identityStore, entry.Token)
		if err == nil {
			return principal, nil
		}
	}
	return core.AuthenticatedPrincipal{
		TenantID:      "default",
		AuthMethod:    core.AuthMethodBootstrapAdmin,
		Authenticated: true,
		Scopes:        []string{"nexus:admin"},
		Subject: core.SubjectRef{
			TenantID: "default",
			Kind:     core.SubjectKindServiceAccount,
			ID:       "local-admin",
		},
	}, nil
}

func newNodePairCmd(workspace, configPath *string) *cobra.Command {
	var deviceID string
	var approveNow bool
	cmd := &cobra.Command{
		Use:   "pair",
		Short: "Create a node pairing request",
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()
			paths, cfg, err := resolveConfig(*workspace, *configPath)
			if err != nil {
				return err
			}
			manager, store, logStore, err := nexusbootstrap.OpenNodeManager(paths, cfg)
			if err != nil {
				return err
			}
			defer store.Close()
			defer logStore.Close()
			identityStore, err := nexusdb.NewSQLiteIdentityStore(paths.IdentityStoreFile())
			if err != nil {
				return err
			}
			defer identityStore.Close()
			if deviceID == "" {
				deviceID = fmt.Sprintf("node-%d", time.Now().UTC().Unix())
			}
			cred, _, err := fwnode.GenerateCredential(deviceID)
			if err != nil {
				return err
			}
			code, err := manager.RequestPairing(ctx, cred)
			if err != nil {
				return err
			}
			if approveNow || cfg.Nodes.AutoApproveLocal {
				pairing, _, _ := manager.PairingStatus(ctx, code)
				if err := manager.ApprovePairing(ctx, code); err != nil {
					return err
				}
				if pairing != nil {
					enrollment := nodeEnrollmentFromPairing(*pairing)
					if err := upsertTenantAndSubject(ctx, identityStore, enrollment.TenantID, enrollment.Owner.Kind, enrollment.Owner.ID, enrollment.Owner.ID, nil, enrollment.PairedAt); err != nil {
						return err
					}
					if err := identityStore.UpsertNodeEnrollment(ctx, enrollment); err != nil {
						return err
					}
					if err := store.UpsertNode(ctx, nodeDescriptorFromEnrollment(enrollment)); err != nil {
						return err
					}
				}
				_, err = fmt.Fprintf(cmd.OutOrStdout(), "Pairing approved for %s with code %s\n", deviceID, code)
				return err
			}
			_, err = fmt.Fprintf(cmd.OutOrStdout(), "Pairing requested for %s with code %s\n", deviceID, code)
			return err
		},
	}
	cmd.Flags().StringVar(&deviceID, "device-id", "", "device id for the node")
	cmd.Flags().BoolVar(&approveNow, "approve-now", false, "approve the request immediately")
	return cmd
}

func newNodeApproveCmd(workspace, configPath *string) *cobra.Command {
	return &cobra.Command{
		Use:   "approve <pairing-code>",
		Short: "Approve a pending node pairing request",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := nexusadmin.ApprovePairing(cmd.Context(), *workspace, *configPath, args[0]); err != nil {
				return err
			}
			_, err := fmt.Fprintf(cmd.OutOrStdout(), "Pairing %s approved\n", args[0])
			return err
		},
	}
}

func newNodeRejectCmd(workspace, configPath *string) *cobra.Command {
	return &cobra.Command{
		Use:   "reject <pairing-code>",
		Short: "Reject a pending node pairing request",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := nexusadmin.RejectPairing(cmd.Context(), *workspace, *configPath, args[0]); err != nil {
				return err
			}
			_, err := fmt.Fprintf(cmd.OutOrStdout(), "Pairing %s rejected\n", args[0])
			return err
		},
	}
}
