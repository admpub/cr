package cr

import (
	"context"
	"errors"
	"fmt"
	"log"
	"strconv"
	"strings"
	"time"

	cdp "github.com/knq/chromedp"
	extras "github.com/knq/chromedp/cdp"
)

// ErrNotFound is returned when an XPATH is provided
// for a DOM element, but it can not be located.
var ErrNotFound = errors.New("element not found")

type action func(interface{}, ...cdp.QueryOption) cdp.Action

// Browser represents a Chrome browser controlled by chromedp.
type Browser struct {
	ctx           context.Context
	cdp           *cdp.CDP
	cancelContext context.CancelFunc
}

// New instantiates a new Chrome browser and returns
// a *Browser used to control it.
func New() (*Browser, error) {
	b := &Browser{}
	ctx, cancel := context.WithCancel(context.Background())

	c, err := cdp.New(ctx)
	if err != nil {
		return b, err
	}

	b.cdp = c
	b.ctx = ctx
	b.cancelContext = cancel

	return b, nil
}

// Close cleans up the *Browser; this should be called
// on every *Browser once its work is complete.
func (b *Browser) Close() error {
	b.cancelContext()
	err := b.cdp.Shutdown(b.ctx)
	if err == nil {
		return err
	}
	return b.cdp.Wait()
}

// Navigate sends the browser to a URL.
func (b *Browser) Navigate(url string) error {
	return b.cdp.Run(b.ctx, cdp.Navigate(url))
}

// Location returns the current URL.
func (b *Browser) Location() (string, error) {
	var location string
	err := b.cdp.Run(b.ctx, cdp.Location(&location))
	return location, err
}

// SendKeys sends keystrokes to a DOM element.
func (b *Browser) SendKeys(xpath, value string) error {
	if err := b.FindElement(xpath); err != nil {
		return err
	}
	return b.cdp.Run(b.ctx, cdp.SendKeys(xpath, value))
}

// Click performs a mouse click on a DOM element.
func (b *Browser) Click(xpath string) error {
	if err := b.FindElement(xpath); err != nil {
		return err
	}
	return b.cdp.Run(b.ctx, cdp.Click(xpath))
}

// GetSource returns the HTML source from the browser tab.
func (b *Browser) GetSource() (string, error) {
	var html string
	err := b.cdp.Run(b.ctx, cdp.OuterHTML("/*", &html))
	return html, err
}

// GetAttributes returns the HTML attributes of a DOM element.
func (b *Browser) GetAttributes(xpath string) (map[string]string, error) {
	attrs := make(map[string]string)
	if err := b.FindElement(xpath); err != nil {
		return attrs, err
	}
	err := b.cdp.Run(b.ctx, cdp.Attributes(xpath, &attrs))
	return attrs, err
}

// ClickByXY clicks the browser window in a specific location.
func (b *Browser) ClickByXY(xpath string) error {
	if err := b.FindElement(xpath); err != nil {
		return err
	}
	log.Printf("sleeping\n")
	time.Sleep(time.Second * 5)
	log.Printf("done sleeping\n")
	x, y, err := b.GetTopLeft(xpath)
	log.Printf("GetTopLeft returned %d %d\n", x, y)
	if err != nil {
		return err
	}
	return b.cdp.Run(b.ctx, cdp.MouseClickXY(x, y))
}

// GetTopLeft returns the x, y coordinates of a DOM element.
func (b *Browser) GetTopLeft(xpath string) (int64, int64, error) {
	var top, left float64
	if err := b.FindElement(xpath); err != nil {
		log.Printf("GetTopLeft couldn't find %s: %s\n", xpath, err)
		return 0, 0, err
	}
	js := fmt.Sprintf(topLeftJS, xpath)
	var result string
	err := b.cdp.Run(b.ctx, cdp.Evaluate(js, &result))
	parts := strings.Split(result, ":")
	if len(parts) == 2 {
		top, err = strconv.ParseFloat(parts[0], 64)
		if err != nil {
			log.Printf("Failed to parse top coordinate: %s\n", err)
			return 0, 0, err
		}
		left, err = strconv.ParseFloat(parts[1], 64)
		if err != nil {
			log.Printf("Failed to parse left coordinate: %s\n", err)
			return 0, 0, err
		}
	}
	log.Printf("GetTopLeft found %.2f %.2f\n", top, left)
	return int64(top) + 1, int64(left) + 1, err
}

// ClickNode clicks an element by node name.
func (b *Browser) ClickNode(xpath string) error {
	var err error
	for i := 0; i < 5; i++ {
		time.Sleep(time.Second)
		if err = b.FindElement(xpath); err != nil {
			log.Printf("failed to find element for %s\n", xpath)
			continue
		}
		var nodes []*extras.Node
		nodes, err = b.GetNodes(xpath)
		if err != nil {
			log.Printf("failed to get node for %s\n", xpath)
			continue
		}
		if len(nodes) < 1 {
			err = ErrNotFound
			continue
		}
		log.Printf("Found %d %q nodes\n", len(nodes), xpath)
		for i, node := range nodes {
			err = b.cdp.Run(b.ctx, cdp.MouseClickNode(node))
			if err != nil {
				log.Printf("Error clicking %s #%d\n", xpath, i+1)
				continue
			}
		}
		log.Printf("clickNode for %s successful on attempt %d\n", xpath, i+1)
		err = nil
		break
	}
	return err
}

// FindElement attempts to locate a DOM element.
func (b *Browser) FindElement(xpath string) error {
	log.Printf("In FindElement for %q\n", xpath)
	wait := time.Millisecond * 100

	for i := 0; i < 8; i++ {
		attempt := i + 1
		time.Sleep(wait)
		log.Printf("FindElement attempt %d %q\n", attempt, xpath)
		nodes, err := b.GetNodes(xpath)
		if err != nil {
			log.Printf("Error getting nodes during attempt %d\n", attempt)
			continue
		}
		if len(nodes) > 0 {
			log.Printf("Attempt %d found %d nodes\n", attempt, len(nodes))
			return nil
		}
		wait = wait * 2
	}
	return ErrNotFound
}

// GetNodes returns a slice of *cdp.Node from the chromedp package.
func (b *Browser) GetNodes(xpath string) ([]*extras.Node, error) {
	var nodes []*extras.Node
	err := b.cdp.Run(b.ctx, cdp.Nodes(xpath, &nodes))
	return nodes, err
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
