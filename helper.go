package cr

import (
	"context"
	"math"

	"github.com/chromedp/cdproto/cdp"
	"github.com/chromedp/cdproto/dom"
	"github.com/chromedp/cdproto/emulation"
	"github.com/chromedp/cdproto/page"
	"github.com/chromedp/chromedp"
)

// fullScreenshot takes a screenshot of the entire browser viewport.
//
// Liberally copied from puppeteer's source.
//
// Note: this will override the viewport emulation settings.
func fullScreenshot(urlStr string, quality int64, res *[]byte) chromedp.Action[chromedp.Void] {
	return chromedp.Steps(
		chromedp.Navigate(urlStr),
		chromedp.Func(func(ctx context.Context, t *chromedp.Target) error {
			// get layout metrics
			metrics, err := cdp.Call(ctx, t, page.GetLayoutMetrics, cdp.Empty{})
			if err != nil {
				return err
			}
			contentSize := metrics.CSSContentSize
			if contentSize == nil {
				contentSize = &dom.Rect{}
			}

			width, height := int64(math.Ceil(contentSize.Width)), int64(math.Ceil(contentSize.Height))

			// force viewport emulation
			_, err = cdp.Call(ctx, t, emulation.SetDeviceMetricsOverride, emulation.SetDeviceMetricsOverrideParams{
				Width:             width,
				Height:            height,
				DeviceScaleFactor: 1,
				Mobile:            false,
				ScreenOrientation: &emulation.ScreenOrientation{
					Type:  emulation.ScreenOrientationTypePortraitPrimary,
					Angle: 0,
				},
			})
			if err != nil {
				return err
			}

			// capture screenshot
			result, err := cdp.Call(ctx, t, page.CaptureScreenshot, page.CaptureScreenshotParams{
				Quality: &quality,
				Clip: &page.Viewport{
					X:      contentSize.X,
					Y:      contentSize.Y,
					Width:  contentSize.Width,
					Height: contentSize.Height,
					Scale:  1,
				},
			})
			if err != nil {
				return err
			}
			*res = result.Data
			return nil
		}),
	)
}
