package server

import (
	"context"
	"fmt"
	"strings"
)

type ResourceDriver interface {
	Kind() string
	Actions() []string
	Test(context.Context, *App, connectedResource) error
	Sync(context.Context, *App, connectedResource) (syncResult, error)
	Execute(context.Context, *App, connectedResource, string, map[string]any) (map[string]any, error)
}

type emailDriver struct{}
type calendarDriver struct{}

func connectedDriverFor(kind string) (ResourceDriver, bool) {
	switch kind {
	case "email.account":
		return emailDriver{}, true
	case "calendar.account":
		return calendarDriver{}, true
	default:
		return nil, false
	}
}

func connectedKindSupported(kind string) bool {
	_, ok := connectedDriverFor(kind)
	return ok
}

func connectedActionSupported(action string) bool {
	if action == "notify" || strings.HasPrefix(action, "todo.") {
		return true
	}
	for _, kind := range []string{"email.account", "calendar.account"} {
		d, _ := connectedDriverFor(kind)
		for _, a := range d.Actions() {
			if a == action {
				return true
			}
		}
	}
	return false
}

func (emailDriver) Kind() string    { return "email.account" }
func (calendarDriver) Kind() string { return "calendar.account" }
func (emailDriver) Actions() []string {
	return []string{"email.send", "email.reply", "email.move", "email.mark_read"}
}
func (calendarDriver) Actions() []string {
	return []string{"calendar.create", "calendar.update", "calendar.delete"}
}

func (emailDriver) Test(ctx context.Context, app *App, r connectedResource) error {
	c, err := connectIMAP(r)
	if err != nil {
		return err
	}
	_ = c.Logout()
	if smtpHost := secretString(r, "smtp_host"); smtpHost != "" {
		return testSMTP(r)
	}
	return nil
}

func (calendarDriver) Test(ctx context.Context, app *App, r connectedResource) error {
	return caldavPropfind(r)
}

func (emailDriver) Sync(ctx context.Context, app *App, r connectedResource) (syncResult, error) {
	return app.syncEmailResource(ctx, r)
}

func (calendarDriver) Sync(ctx context.Context, app *App, r connectedResource) (syncResult, error) {
	return app.syncCalDAVResource(ctx, r)
}

func (emailDriver) Execute(ctx context.Context, app *App, r connectedResource, action string, input map[string]any) (map[string]any, error) {
	return app.executeEmailAction(ctx, r, action, input)
}

func (calendarDriver) Execute(ctx context.Context, app *App, r connectedResource, action string, input map[string]any) (map[string]any, error) {
	return app.executeCalendarAction(ctx, r, action, input)
}

func executeResourceAction(ctx context.Context, app *App, r connectedResource, action string, input map[string]any) (map[string]any, error) {
	d, ok := connectedDriverFor(r.Kind)
	if !ok {
		return nil, fmt.Errorf("unsupported connected resource kind %s", r.Kind)
	}
	return d.Execute(ctx, app, r, action, input)
}
