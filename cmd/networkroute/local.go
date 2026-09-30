package main

import (
	"context"
	"flag"
	"fmt"
	"networkroute/internal/console"
	"networkroute/internal/localnet"
	"networkroute/internal/monitor"
	"networkroute/internal/startup"
	"os"
	"os/signal"
	"syscall"
)

func localCommand(command string, args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("%s requires an action (see help)", command)
	}
	action := args[0]
	fs := flag.NewFlagSet(command, flag.ContinueOnError)
	dir := fs.String("data-dir", "", "settings directory")
	if err := fs.Parse(args[1:]); err != nil {
		return err
	}
	if fs.NArg() != 0 {
		return fmt.Errorf("unexpected arguments")
	}
	if *dir == "" {
		var err error
		*dir, err = console.DefaultDirectory()
		if err != nil {
			return err
		}
	}
	if command == "autostart" {
		switch action {
		case "enable":
			if err := startup.Enable(*dir); err != nil {
				return err
			}
		case "disable":
			if err := startup.Disable(*dir); err != nil {
				return err
			}
		case "status":
		default:
			return fmt.Errorf("autostart actions: enable, disable, status")
		}
		s, err := startup.Status(*dir)
		if err != nil {
			return err
		}
		return output(s)
	}
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	// Recovery must remain available even if the saved settings are damaged.
	if action == "remove" || action == "status" {
		r, err := localnet.QoS(ctx, *dir, action, nil)
		if err != nil { return err }; fmt.Println(r); return nil
	}
	s, err := localnet.Load(*dir)
	if err != nil {
		return err
	}
	switch action {
	case "settings":
		return output(s)
	case "traffic":
		r, err := monitor.TrafficRates(ctx)
		if err != nil {
			return err
		}
		return output(r)
	case "apply", "remove", "status":
		r, err := localnet.QoS(ctx, *dir, action, s.Rules)
		if err != nil {
			return err
		}
		fmt.Println(r)
		return nil
	case "before", "after":
		endpoint := ""
		var before localnet.Report
		if action == "after" {
			before, err = localnet.LoadBaseline(*dir)
			if err != nil {
				return err
			}
			if before.Target != s.Target {
				return fmt.Errorf("target changed; repeat baseline")
			}
			endpoint = before.Endpoint
		}
		r, err := localnet.Measure(ctx, s, endpoint)
		if err != nil {
			return err
		}
		if err := localnet.SaveReport(*dir, action, r); err != nil {
			return err
		}
		verdict := "baseline saved"
		if action == "after" {
			verdict = localnet.Compare(before, r)
		}
		return output(map[string]any{"report": r, "verdict": verdict})
	default:
		return fmt.Errorf("local actions: settings, traffic, status, apply, remove, before, after")
	}
}
