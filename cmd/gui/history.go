package gui

import (
	l "gioui.org/layout"
	
	"github.com/p9c/pod/pkg/gui/p9"
)

func (wg *WalletGUI) HistoryPage() l.Widget {
	
	return func(gtx l.Context) l.Dimensions {
		return wg.th.VFlex().
			Rigid(
				wg.th.Inset(0.25,
					wg.th.Fill("DocBg", wg.th.Responsive(*wg.Size, p9.Widgets{
						{
							Widget: wg.th.VFlex().
								Flexed(1, wg.HistoryPageView()).
								Rigid(
									// 	wg.th.Fill("DocBg",
									wg.th.Flex().AlignMiddle().SpaceBetween().
										Flexed(0.5, p9.EmptyMaxWidth()).
										Rigid(wg.HistoryPageStatusFilter()).
										Flexed(0.5, p9.EmptyMaxWidth()).
										Fn,
									// 	).Fn,
								).
								// Rigid(
								// 	wg.th.Fill("DocBg",
								// 		wg.th.Flex().AlignMiddle().SpaceBetween().
								// 			Rigid(wg.HistoryPager()).
								// 			Rigid(wg.HistoryPagePerPageCount()).
								// 			Fn,
								// 	).Fn,
								// ).
								Fn,
						},
						{
							Size: 1280,
							Widget: wg.th.VFlex().
								Flexed(1, wg.HistoryPageView()).
								Rigid(
									// 	wg.th.Fill("DocBg",
									wg.th.Flex().AlignMiddle().SpaceBetween().
										// 			Rigid(wg.HistoryPager()).
										Flexed(0.5, p9.EmptyMaxWidth()).
										Rigid(wg.HistoryPageStatusFilter()).
										Flexed(0.5, p9.EmptyMaxWidth()).
										// 			Rigid(wg.HistoryPagePerPageCount()).
										Fn,
									// 	).Fn,
								).
								Fn,
						},
					}).Fn, l.Center).Fn,
				).Fn,
			).Fn(gtx)
	}
}

func (wg *WalletGUI) HistoryPageView() l.Widget {
	return wg.th.VFlex().Rigid(wg.RecentTransactionsWidget).Fn
}

func (wg *WalletGUI) HistoryPageStatusFilter() l.Widget {
	return wg.th.Flex().AlignMiddle().
		Rigid(
			wg.th.Inset(0.25,
				wg.th.Caption("show").Fn,
			).Fn,
		).
		Rigid(
			wg.th.Inset(0.25,
				func(gtx l.Context) l.Dimensions {
					return wg.th.CheckBox(wg.bools["showGenerate"]).
						TextColor("DocText").
						TextScale(1).
						Text("generate").
						IconScale(1).
						Fn(gtx)
				},
			).Fn,
		).
		Rigid(
			wg.th.Inset(0.25,
				func(gtx l.Context) l.Dimensions {
					return wg.th.CheckBox(wg.bools["showSent"]).
						TextColor("DocText").
						TextScale(1).
						Text("sent").
						IconScale(1).
						Fn(gtx)
				},
			).Fn,
		).
		Rigid(
			wg.th.Inset(0.25,
				func(gtx l.Context) l.Dimensions {
					return wg.th.CheckBox(wg.bools["showReceived"]).
						TextColor("DocText").
						TextScale(1).
						Text("received").
						IconScale(1).
						Fn(gtx)
				},
			).Fn,
		).
		Rigid(
			wg.th.Inset(0.25,
				func(gtx l.Context) l.Dimensions {
					return wg.th.CheckBox(wg.bools["showImmature"]).
						TextColor("DocText").
						TextScale(1).
						Text("immature").
						IconScale(1).
						Fn(gtx)
				},
			).Fn,
		).
		Fn
}
