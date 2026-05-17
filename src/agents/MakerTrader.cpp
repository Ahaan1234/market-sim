#include "MakerTrader.h"

MakerTrader::MakerTrader(std::string id, double capital, unsigned int seed,
                         double offset, double qty)
    : Trader(std::move(id), capital, seed), offset_(offset), qty_(qty) {}

void MakerTrader::act(OrderBook& book, double midPrice, uint64_t tick) {
    (void)tick;
    if (book.spread() > 2.0 * offset_ * midPrice * (1.0 + 1e-6))
        return;

    book.addLimitOrder(Side::BUY,  midPrice * (1.0 - offset_), qty_);
    book.addLimitOrder(Side::SELL, midPrice * (1.0 + offset_), qty_);
}
