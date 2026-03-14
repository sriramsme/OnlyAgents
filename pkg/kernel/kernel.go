// Kernel is the composition root and central event router.
//
// It owns all registries and is the ONLY package that imports agents, skills,
// connectors, and channels together. Nothing else does.
//
// Event flow:
//   Channel → bus (MessageReceived)
//   Kernel  → agent.inbox (AgentExecute)
//   Agent   → bus (ToolCallRequest)
//   Kernel  → skill.Execute() → replies via ReplyTo channel
//   Agent   → bus (OutboundMessage)
//   Kernel  → channel.Send()

package kernel

import (
	"context"
	"fmt"
	"log/slog"
	"sync"

	_ "modernc.org/sqlite"

	"github.com/sriramsme/OnlyAgents/internal/bootstrap"
	"github.com/sriramsme/OnlyAgents/internal/config"
	"github.com/sriramsme/OnlyAgents/pkg/agents"
	"github.com/sriramsme/OnlyAgents/pkg/channels"
	"github.com/sriramsme/OnlyAgents/pkg/connectors"
	"github.com/sriramsme/OnlyAgents/pkg/core"
	"github.com/sriramsme/OnlyAgents/pkg/llm"
	"github.com/sriramsme/OnlyAgents/pkg/memory"
	"github.com/sriramsme/OnlyAgents/pkg/skills"
	"github.com/sriramsme/OnlyAgents/pkg/skills/cli"
	"github.com/sriramsme/OnlyAgents/pkg/skills/marketplace"
	"github.com/sriramsme/OnlyAgents/pkg/storage"
	"github.com/sriramsme/OnlyAgents/pkg/workflow"
)

// Kernel is the central router. It wires everything together and owns the event bus.
type Kernel struct {
	bus        chan core.Event
	agents     *agents.Registry
	skills     *skills.Registry
	connectors *connectors.Registry
	channels   *channels.Registry
	workflow   *workflow.Engine
	user       *config.UserConfig

	skillMarketplaceManager *marketplace.Manager
	cliExecutor             *cli.CLIExecutor
	cm                      *memory.ConversationManager
	mm                      *memory.MemoryManager
	store                   storage.Storage

	// helperClient is used for skill installation
	helperClient llm.Client

	cfg *config.AppConfig

	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup
	logger *slog.Logger

	// UI fan-out — nil when running headless (no GUI/server).
	// Agents write UIEvents here;
	uiBus core.UIBus
}

func NewKernel(ctx context.Context, cancel context.CancelFunc, uiBus core.UIBus) (*Kernel, error) {
	paths, err := bootstrap.Init()
	if err != nil {
		return nil, fmt.Errorf("init paths: %w", err)
	}

	cfg, err := config.LoadAppConfig()
	if err != nil {
		return nil, fmt.Errorf("load application config: %w", err)
	}

	kernelBus := make(chan core.Event, cfg.BusBufferSize)

	components, err := loadComponents(ctx, paths, cfg, kernelBus, uiBus)
	if err != nil {
		return nil, err
	}

	fmt.Println("kernel created")
	fmt.Println("skills: ", components.skills.ListAll())
	fmt.Println("agents: ", components.agents.ListAll())

	return &Kernel{
		bus:                     kernelBus,
		agents:                  components.agents,
		skills:                  components.skills,
		connectors:              components.connectors,
		channels:                components.channels,
		user:                    components.user,
		workflow:                components.workflow,
		skillMarketplaceManager: components.skillMarketplaceManager,
		cliExecutor:             components.cliExecutor,
		cm:                      components.cm,
		mm:                      components.mm,
		store:                   components.store,
		// helperClient:            components.helperClient,
		cfg:    cfg,
		ctx:    ctx,
		cancel: cancel,
		logger: slog.Default().With("component", "kernel"),
		uiBus:  uiBus,
	}, nil
}

// --- Registration (called during app bootstrap) ---

func (k *Kernel) RegisterAgent(a *agents.Agent) {
	k.agents.Register(a)
}

func (k *Kernel) RegisterSkill(s config.SkillConfig) {
	k.skills.Register(s)
}

func (k *Kernel) RegisterConnector(c connectors.Connector) {
	k.connectors.Register(c)
}

func (k *Kernel) RegisterChannel(ch channels.Channel) {
	k.channels.Register(ch)
}

// Bus returns the write-end of the event bus.
// Channels and agents write here; kernel reads and routes.
func (k *Kernel) Bus() chan<- core.Event {
	return k.bus
}
