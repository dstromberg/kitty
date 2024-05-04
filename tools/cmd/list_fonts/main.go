package list_fonts

import (
	"encoding/json"
	"fmt"
	"os"
	"sync"

	"kitty/tools/cli"
	"kitty/tools/tty"
	"kitty/tools/tui/loop"
)

var _ = fmt.Print
var debugprintln = tty.DebugPrintln

var json_decoder *json.Decoder

func json_decode(v any) error {
	if err := json_decoder.Decode(v); err != nil {
		return fmt.Errorf("Failed to decode JSON from kitty with error: %w", err)
	}
	return nil
}

func to_kitty(v any) error {
	data, err := json.Marshal(v)
	if err != nil {
		return fmt.Errorf("Could not encode message to kitty with error: %w", err)
	}
	if _, err = os.Stdout.Write(data); err != nil {
		return fmt.Errorf("Failed to send message to kitty with I/O error: %w", err)
	}
	if _, err = os.Stdout.WriteString("\n"); err != nil {
		return fmt.Errorf("Failed to send message to kitty with I/O error: %w", err)
	}
	return nil
}

var query_kitty_lock sync.Mutex

func query_kitty(action string, cmd map[string]any, result any) error {
	query_kitty_lock.Lock()
	defer query_kitty_lock.Unlock()
	if action != "" {
		cmd["action"] = action
		if err := to_kitty(cmd); err != nil {
			return err
		}
	}
	return json_decode(result)
}

func main() (rc int, err error) {
	json_decoder = json.NewDecoder(os.Stdin)
	lp, err := loop.New()
	if err != nil {
		return 1, err
	}
	h := &handler{lp: lp}
	lp.OnInitialize = func() (string, error) {
		lp.AllowLineWrapping(false)
		lp.SetWindowTitle(`Choose a font for kitty`)
		h.initialize()
		return "", nil
	}
	lp.OnWakeup = h.on_wakeup
	lp.OnFinalize = func() string {
		h.finalize()
		lp.SetCursorVisible(true)
		return ``
	}
	lp.OnMouseEvent = h.on_mouse_event
	lp.OnResize = func(_, _ loop.ScreenSize) error {
		return h.draw_screen()
	}
	lp.OnKeyEvent = h.on_key_event
	lp.OnText = h.on_text
	err = lp.Run()
	if err != nil {
		return 1, err
	}
	ds := lp.DeathSignalName()
	if ds != "" {
		fmt.Println("Killed by signal: ", ds)
		lp.KillIfSignalled()
		return 1, nil
	}
	return lp.ExitCode(), nil
}

func EntryPoint(root *cli.Command) {
	root = root.AddSubCommand(&cli.Command{
		Name:   "__list_fonts__",
		Hidden: true,
		Run: func(cmd *cli.Command, args []string) (rc int, err error) {
			return main()
		},
	})
}
