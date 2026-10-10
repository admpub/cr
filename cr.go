package cr

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/admpub/log"
	"github.com/chromedp/chromedp"
)

// ErrNotFound is returned when an XPATH is provided
// for a DOM element, but it can not be located.
var ErrNotFound = errors.New("element not found")

const minTimeout = time.Second

// Browser represents a Chrome browser controlled by chromedp.
type Browser struct {
	allocCtx       context.Context
	allocCtxCancel context.CancelFunc
	taskCtx        context.Context
	taskCtxCancel  context.CancelFunc
	timeout        time.Duration
	logger         *log.Logger
}

// New instantiates a new Chrome browser and returns
// a *Browser used to control it.
func New(ctx context.Context, args ...chromedp.ExecAllocatorOption) (*Browser, error) {
	b := &Browser{
		timeout: time.Second * 30,
		logger:  log.GetLogger(`ChromeDP`),
	}
	options := append(chromedp.DefaultExecAllocatorOptions[:],
		chromedp.DisableGPU,
		chromedp.Headless,
	)
	options = append(options, args...)
	b.allocCtx, b.allocCtxCancel = chromedp.NewExecAllocator(ctx, options...)

	// also set up a custom logger
	b.taskCtx, b.taskCtxCancel = chromedp.NewContext(b.allocCtx, chromedp.WithLogf(b.logger.Errorf))

	// ensure that the browser process is started
	if err := chromedp.Do(b.taskCtx); err != nil {
		b.Close()
		return b, err
	}

	return b, nil
}

// SetTimeout accepts a time.Duration. This duration will
// be used as the maximum timeout when waiting for a node to exist.
func (b *Browser) SetTimeout(d time.Duration) {
	if d < minTimeout {
		d = minTimeout
	}
	b.timeout = d
}

// Close cleans up the *Browser; this should be called
// on every *Browser once its work is complete.
func (b *Browser) Close() error {
	b.allocCtxCancel()
	b.taskCtxCancel()
	return nil
}

// context returns the task context, with the timeout applied if set.
func (b *Browser) context() (context.Context, context.CancelFunc) {
	if b.timeout > 0 {
		return context.WithTimeout(b.taskCtx, b.timeout)
	}
	return b.taskCtx, func() {}
}

// RunAction run single action
func (b *Browser) RunAction(action chromedp.Action[chromedp.Void]) error {
	ctx, cancel := b.context()
	defer cancel()
	_, err := chromedp.Run(ctx, action)
	return err
}

// RunTasks run multiple actions
func (b *Browser) RunTasks(actions ...chromedp.Action[chromedp.Void]) error {
	return b.RunAction(chromedp.Steps(actions...))
}

// RunTaskWithOther run multiple actions
func (b *Browser) RunTaskWithOther(action chromedp.Action[chromedp.Void], otherActions ...chromedp.Action[chromedp.Void]) error {
	actions := append([]chromedp.Action[chromedp.Void]{action}, otherActions...)
	return b.RunTasks(actions...)
}

// Navigate sends the browser to a URL.
func (b *Browser) Navigate(url string, otherActions ...chromedp.Action[chromedp.Void]) error {
	return b.RunTaskWithOther(chromedp.Navigate(url), otherActions...)
}

// MustNavigate calls Navigate and ends execution on error.
func (b *Browser) MustNavigate(url string, otherActions ...chromedp.Action[chromedp.Void]) {
	if err := b.Navigate(url, otherActions...); err != nil {
		log.Fatalf("Failed to navigate to %q: %s\n", url, err)
	}
}

// Location returns the current URL.
func (b *Browser) Location(otherActions ...chromedp.Action[chromedp.Void]) (string, error) {
	if len(otherActions) > 0 {
		if err := b.RunTasks(otherActions...); err != nil {
			return "", err
		}
	}
	ctx, cancel := b.context()
	defer cancel()
	return chromedp.Run(ctx, chromedp.Location())
}

// SendKeys sends keystrokes to a DOM element.
func (b *Browser) SendKeys(xpath, value string) error {
	return b.RunAction(chromedp.SendKeys(xpath, value))
}

// MustSendKeys sends keystrokes to a DOM element or halts execution.
func (b *Browser) MustSendKeys(xpath, value string) {
	if err := b.SendKeys(xpath, value); err != nil {
		log.Fatalf("Failed to send %q to %q: %s\n", value, xpath, err)
	}
}

// Click performs a mouse click on a DOM element.
func (b *Browser) Click(xpath string) error {
	return b.RunAction(chromedp.Click(xpath))
}

// MustClick performs a mouse click or ends the program.
func (b *Browser) MustClick(xpath string) {
	if err := b.Click(xpath); err != nil {
		log.Fatalf("Failed to click %q: %s\n", xpath, err)
	}
}

// GetSource returns the HTML source from the browser tab.
func (b *Browser) GetSource() (string, error) {
	ctx, cancel := b.context()
	defer cancel()
	return chromedp.Run(ctx, chromedp.OuterHTML(chromedp.CSS("html")))
}

// GetAttributes returns the HTML attributes of a DOM element.
func (b *Browser) GetAttributes(xpath string) (map[string]string, error) {
	ctx, cancel := b.context()
	defer cancel()
	return chromedp.Run(ctx, chromedp.Attributes(xpath))
}

// ClickByXY clicks the browser window in a specific location.
func (b *Browser) ClickByXY(xpath string) error {
	x, y, err := b.GetTopLeft(xpath)
	if err != nil {
		return err
	}
	return b.RunAction(chromedp.MouseClickXY(x, y))
}

// GetTopLeft returns the x, y coordinates of a DOM element.
func (b *Browser) GetTopLeft(xpath string) (float64, float64, error) {
	js := fmt.Sprintf(topLeftJS, xpath)
	ctx, cancel := b.context()
	defer cancel()
	result, err := chromedp.Run(ctx, chromedp.Evaluate[string](js))
	if err != nil {
		return 0, 0, err
	}
	var top, left float64
	parts := strings.Split(result, ":")
	if len(parts) == 2 {
		top, err = strconv.ParseFloat(parts[0], 64)
		if err != nil {
			b.logger.Errorf("Failed to parse top coordinate: %s", err)
			return 0, 0, err
		}
		left, err = strconv.ParseFloat(parts[1], 64)
		if err != nil {
			b.logger.Errorf("Failed to parse left coordinate: %s", err)
			return 0, 0, err
		}
	}
	return top + 1, left + 1, err
}

func (b *Browser) ElementScreenshot(urlStr string, selectionElem string, by ...chromedp.QueryOption) ([]byte, error) {
	// keep the old default: a plain string selects the element by its id
	sel := chromedp.ID(selectionElem)
	opts := append([]chromedp.QueryOption(nil), by...)
	if err := b.RunTasks(
		chromedp.Navigate(urlStr),
		chromedp.WaitVisible(sel, opts...),
	); err != nil {
		return nil, err
	}
	ctx, cancel := b.context()
	defer cancel()
	return chromedp.Run(ctx, chromedp.Screenshot(sel, opts...))
}

func (b *Browser) Screenshot(urlStr string, quality int64) ([]byte, error) {
	var buf []byte
	err := b.RunAction(fullScreenshot(urlStr, quality, &buf))
	return buf, err
}

// FindElement attempts to locate a DOM element.
func (b *Browser) FindElement(xpath string) error {
	nodes, err := b.GetNodes(xpath)
	if err != nil {
		return err
	}
	if len(nodes) > 0 {
		return nil
	}
	return ErrNotFound
}

// GetNodes returns a slice of *chromedp.Node from the chromedp package.
func (b *Browser) GetNodes(xpath string) ([]*chromedp.Node, error) {
	ctx, cancel := b.context()
	defer cancel()
	return chromedp.Run(ctx, chromedp.Nodes(xpath))
}

var topLeftJS = `
	function getTopLeft() {
		var element = document.evaluate("%s",document, null, XPathResult.FIRST_ORDERED_NODE_TYPE, null ).singleNodeValue;
		var rect = element.getBoundingClientRect();
		return rect.top + ":" + rect.left;
	}
	(function main() {
	   return getTopLeft();
	})(); 
	`
