#include "MakerTrader.h"

MakerTrader::MakerTrader(std::string id, double capital, unsigned int seed,
                         double offset, double qty)
    : Trader(std::move(id), capital, seed), offset_(offset), qty_(qty) {}

void MakerTrader::act(OrderBook& book, double midPrice, uint64_t tick) {
    (void)tick;

    if (prevBid_ >= 0.0) book.removeQty(Side::BUY,  prevBid_, qty_);
    if (prevAsk_ >= 0.0) book.removeQty(Side::SELL, prevAsk_, qty_);
    prevBid_ = prevAsk_ = -1.0;

    if (book.spread() > 2.0 * offset_ * midPrice * (1.0 + 1e-6))
        return;

    prevBid_ = midPrice * (1.0 - offset_);
    prevAsk_ = midPrice * (1.0 + offset_);
    book.addLimitOrder(Side::BUY,  prevBid_, qty_);
    book.addLimitOrder(Side::SELL, prevAsk_, qty_);
}
