// Command plandriver parks a dev game at a chosen plan's resolution, so the
// twelve plans can be looked at from the same starting board.
//
// Setting a plan's *resolution* up by hand is a dozen requests, and every plan
// wants a different prepare payload. Done twelve different ways it stops being
// comparable, which is the whole point of an audit. This does it one way.
//
//	go run ./cmd/plandriver setup spread_propaganda -force mar
//	go run ./cmd/plandriver setup chronicle_histories
//	go run ./cmd/plandriver force 301 mar
//	go run ./cmd/plandriver cleanup 301
//
// Nine of the twelve plans open a pre-roll step before any dice exist, so
// `setup -force` has nothing to act on for them — drive the pre-roll through
// the UI (that step is usually the thing worth looking at), then run `force`
// once a roll is live. setup says which kind of plan you got.
//
// Dev-only: needs UNEASY_DEV=1 on the target server, which docker-compose
// already sets for :8080. See docs/DEVELOPMENT.md.
package main

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"strconv"
	"strings"
)

const (
	defaultBase    = "http://localhost:8080"
	defaultPlayers = "alice,bob,carol"
)

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(1)
	}
	var err error
	switch os.Args[1] {
	case "setup":
		err = cmdSetup(os.Args[2:])
	case "force":
		err = cmdForce(os.Args[2:])
	case "cleanup":
		err = cmdCleanup(os.Args[2:])
	case "-h", "--help", "help":
		usage()
		return
	default:
		err = fmt.Errorf("unknown command %q", os.Args[1])
	}
	if err != nil {
		fmt.Fprintf(os.Stderr, "\n✗ %v\n\n", err)
		os.Exit(1)
	}
}

func usage() {
	fmt.Fprintf(os.Stderr, `plandriver — park a dev game at a plan's resolution

  setup <plan_type> [-force make|mar] [-result N]
  force <game_id> <make|mar> [-result N]
  cleanup <game_id>

Common flags: -base %s  -players %s

Plan types:
  %s
`, defaultBase, defaultPlayers, strings.Join(allPlans, "\n  "))
}

// commonFlags registers the flags every subcommand shares.
func commonFlags(fs *flag.FlagSet) (base, players *string) {
	base = fs.String("base", defaultBase, "server base URL")
	players = fs.String("players", defaultPlayers, "comma-separated; the first one prepares")
	return base, players
}

// loginAll opens a session per named player. Each gets its own cookie jar:
// one shared jar would mean every login replaced the last.
func loginAll(base string, names []string) (map[string]*client, error) {
	out := make(map[string]*client, len(names))
	for _, n := range names {
		c, err := newClient(base, n)
		if err != nil {
			return nil, err
		}
		if err := c.login(); err != nil {
			return nil, err
		}
		out[n] = c
	}
	return out, nil
}

func cmdSetup(args []string) error {
	if len(args) == 0 {
		return errors.New("usage: setup <plan_type> [-force make|mar]")
	}
	planType := args[0]
	if _, ok := prepNotes[planType]; !ok {
		return fmt.Errorf("unknown plan type %q (see --help)", planType)
	}

	fs := flag.NewFlagSet("setup", flag.ExitOnError)
	base, playersFlag := commonFlags(fs)
	force := fs.String("force", "", `also force the outcome: "make" or "mar"`)
	result := fs.Int("result", 0, "exact distinct-face count (0 = derive from -force)")
	if err := fs.Parse(args[1:]); err != nil {
		return err
	}
	names := strings.Split(*playersFlag, ",")
	switch {
	case *force != "" && *force != "make" && *force != "mar":
		return errors.New(`-force must be "make" or "mar"`)
	case len(names) < 2:
		return errors.New("need at least two players")
	}
	clients, seeded, assets, err := seedGame(*base, names)
	if err != nil {
		return err
	}
	gameID := seeded.GameID

	var warnings []string
	warn := func(m string) { warnings = append(warnings, m) }

	preparerName, preparer, p, err := preparePlan(
		planType, gameID, names, clients, seeded.Players, assets, warn)
	if err != nil {
		return err
	}

	fmt.Fprintf(os.Stdout, "\n  game          %d\n", gameID)
	fmt.Fprintf(os.Stdout, "  plan          %s #%d\n", planType, p.ID)
	fmt.Fprintf(os.Stdout, "  preparer      %s\n", preparerName)
	fmt.Fprintf(os.Stdout, "  row           %s\n", rowLabel(p.RowNumber))

	if p.RowNumber == nil {
		// Make War / Clandestinely Liaise: the row comes from a simultaneous
		// die reveal every participant must answer, so there is nothing to
		// fast-forward to yet.
		fmt.Fprintf(os.Stdout, "\n  → open %s/table/%d and answer the delay reveal as each player;\n"+
			"    the row is set once everyone has revealed.\n", *base, gameID)
		finish(*base, gameID, warnings)
		return nil
	}

	if err := preparer.post("/api/dev/advance-row", map[string]any{"plan_id": p.ID}, nil); err != nil {
		return err
	}
	fmt.Fprintf(os.Stdout, "  record        advanced to row %d; plan is resolving\n", *p.RowNumber)

	if err := reportOutcome(planType, gameID, preparer, *force, *result); err != nil {
		return err
	}
	finish(*base, gameID, warnings)
	return nil
}

// reportOutcome either names the plan's resolution shape, or forces the roll
// when the caller asked for an outcome and the plan has dice to force.
func reportOutcome(planType string, gameID int64, preparer *client, force string, result int) error {
	switch {
	case force == "":
		shape := "pre-roll step first"
		if rollsImmediately[planType] {
			shape = "rolls immediately"
		}
		fmt.Fprintf(os.Stdout, "  shape         %s\n", shape)
	case !rollsImmediately[planType]:
		fmt.Fprintf(os.Stdout, "\n  ⚠ %s opens a PRE-ROLL step — no dice exist yet, so -force has\n"+
			"    nothing to act on. Drive the pre-roll first, then:\n"+
			"      go run ./cmd/plandriver force %d %s\n", planType, gameID, force)
	default:
		roll, err := forceActiveRoll(preparer, gameID, force, result)
		if err != nil {
			return err
		}
		printRoll(roll)
	}
	return nil
}

// seedGame logs everyone in, seeds a three-player main-event game at row 1,
// and reads back the starting retinues.
func seedGame(base string, names []string) (
	map[string]*client, seedResponse, []asset, error,
) {
	var seeded seedResponse
	clients, err := loginAll(base, names)
	if err != nil {
		return nil, seeded, nil, err
	}
	lead := clients[names[0]]
	if err := lead.post("/api/dev/seed", map[string]any{
		"phase": "main_event", "players": names, "current_row": 1,
	}, &seeded); err != nil {
		return nil, seeded, nil, err
	}
	var resp struct {
		Assets []asset `json:"assets"`
	}
	if err := lead.get(fmt.Sprintf("/api/tables/%d/assets", seeded.GameID), &resp); err != nil {
		return nil, seeded, nil, err
	}
	return clients, seeded, resp.Assets, nil
}

// preparePlan puts the plan on the record and returns who prepared it.
//
// Only the focus player may prepare, and a successful prepare passes focus to
// the next player — so who prepares is decided by the order of these calls,
// not by who we would like it to be.
//
// Make Demands therefore takes two turns: it targets a plan already on the
// record and may not target one of your own. The first player lays the target
// down, which hands focus on, and the second demands it.
func preparePlan(
	planType string,
	gameID int64,
	names []string,
	clients map[string]*client,
	players []player,
	assets []asset,
	warn func(string),
) (string, *client, plan, error) {
	preparerIdx := 0
	var targetPlanID int64

	if planType == "make_demands" {
		var resp struct {
			Plan plan `json:"plan"`
		}
		if err := clients[names[0]].post(
			fmt.Sprintf("/api/tables/%d/prepare-plan", gameID), map[string]any{
				"plan_type": "seek_answers", "preparation_notes": "A quiet inquiry.",
			}, &resp); err != nil {
			return "", nil, plan{}, err
		}
		targetPlanID = resp.Plan.ID
		preparerIdx = 1
		fmt.Fprintf(os.Stdout, "  target plan   seek_answers #%d by %s (row %s)\n",
			targetPlanID, names[0], rowLabel(resp.Plan.RowNumber))
	}

	// buildPrep aims at players[1], which would be the demander themselves
	// here; rotate so Make Demands still points at somebody else.
	ordered := append(append([]player{}, players[preparerIdx:]...), players[:preparerIdx]...)

	body, err := buildPrep(planType, ordered, assets, warn)
	if err != nil {
		return "", nil, plan{}, err
	}
	if targetPlanID != 0 {
		body["target_plan_id"] = targetPlanID
	}

	preparer := clients[names[preparerIdx]]
	var prepared struct {
		Plan plan `json:"plan"`
	}
	if err := preparer.post(
		fmt.Sprintf("/api/tables/%d/prepare-plan", gameID), body, &prepared); err != nil {
		return "", nil, plan{}, err
	}
	return names[preparerIdx], preparer, prepared.Plan, nil
}

func cmdForce(args []string) error {
	if len(args) < 2 {
		return errors.New("usage: force <game_id> <make|mar> [-result N]")
	}
	gameID, err := strconv.ParseInt(args[0], 10, 64)
	if err != nil {
		return fmt.Errorf("invalid game id %q", args[0])
	}
	outcome := args[1]
	if outcome != "make" && outcome != "mar" {
		return errors.New(`outcome must be "make" or "mar"`)
	}

	fs := flag.NewFlagSet("force", flag.ExitOnError)
	base, playersFlag := commonFlags(fs)
	result := fs.Int("result", 0, "exact distinct-face count (0 = derive from the outcome)")
	if err := fs.Parse(args[2:]); err != nil {
		return err
	}

	c, err := newClient(*base, strings.Split(*playersFlag, ",")[0])
	if err != nil {
		return err
	}
	if err := c.login(); err != nil {
		return err
	}
	roll, err := forceActiveRoll(c, gameID, outcome, *result)
	if err != nil {
		return err
	}
	printRoll(roll)
	return nil
}

func cmdCleanup(args []string) error {
	if len(args) < 1 {
		return errors.New("usage: cleanup <game_id>")
	}
	gameID, err := strconv.ParseInt(args[0], 10, 64)
	if err != nil {
		return fmt.Errorf("invalid game id %q", args[0])
	}
	fs := flag.NewFlagSet("cleanup", flag.ExitOnError)
	base, playersFlag := commonFlags(fs)
	if err := fs.Parse(args[1:]); err != nil {
		return err
	}
	c, err := newClient(*base, strings.Split(*playersFlag, ",")[0])
	if err != nil {
		return err
	}
	if err := c.login(); err != nil {
		return err
	}
	if err := c.post("/api/dev/delete-game", map[string]any{"game_id": gameID}, nil); err != nil {
		return err
	}
	fmt.Fprintf(os.Stdout, "  deleted game %d\n", gameID)
	return nil
}

// forceActiveRoll finds the game's open roll and forces it to an outcome.
func forceActiveRoll(c *client, gameID int64, outcome string, result int) (*diceRoll, error) {
	var active struct {
		Roll *diceRoll `json:"roll"`
	}
	if err := c.get(fmt.Sprintf("/api/tables/%d/rolls/active", gameID), &active); err != nil {
		return nil, err
	}
	if active.Roll == nil {
		return nil, fmt.Errorf(
			"no open roll on game %d — if the plan has a pre-roll step, finish it in the UI first",
			gameID)
	}
	body := map[string]any{"roll_id": active.Roll.ID, "outcome": outcome}
	if result > 0 {
		body["result"] = result
	}
	var forced struct {
		Roll diceRoll `json:"roll"`
	}
	if err := c.post("/api/dev/force-roll", body, &forced); err != nil {
		return nil, err
	}
	return &forced.Roll, nil
}

func rowLabel(row *int16) string {
	if row == nil {
		return "— (set by a delay reveal)"
	}
	return strconv.Itoa(int(*row))
}

func printRoll(r *diceRoll) {
	res, out := "—", "—"
	if r.Result != nil {
		res = strconv.Itoa(int(*r.Result))
	}
	if r.Outcome != nil {
		out = *r.Outcome
	}
	fmt.Fprintf(os.Stdout, "  roll          #%d → result %s, outcome %s\n", r.ID, res, out)
}

func finish(base string, gameID int64, warnings []string) {
	for _, w := range warnings {
		fmt.Fprintf(os.Stdout, "\n  ⚠ %s\n", w)
	}
	fmt.Fprintf(os.Stdout, "\n  open          %s/table/%d\n", base, gameID)
	fmt.Fprintf(os.Stdout, "  cleanup       go run ./cmd/plandriver cleanup %d\n\n", gameID)
}
