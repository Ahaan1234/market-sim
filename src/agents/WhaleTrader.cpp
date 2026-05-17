#include "WhaleTrader.h"

WhaleTrader::WhaleTrader(std::string id, double capital, unsigned int seed,
                         int interval, int jitter, double baseQty)
    : Trader(std::move(id), capital, seed)
    , interval_(interval)
    , jitter_(jitter)
    , qty_(15.0 * baseQty)
    , nextActTick_(0)
    , jitterDist_(-jitter, jitter)
    , sideDist_(0, 1)
{
    nextActTick_ = computeNextTick(0);
}

uint64_t WhaleTrader::computeNextTick(uint64_t currentTick) {
    int offset = interval_ + jitterDist_(rng_);
    if (offset < 1) offset = 1;
    return currentTick + static_cast<uint64_t>(offset);
}

void WhaleTrader::act(OrderBook& book, double midPrice, uint64_t tick) {
    (void)midPrice;
    if (tick < nextActTick_)
        return;

    Side side = (sideDist_(rng_) == 0) ? Side::BUY : Side::SELL;
    book.addMarketOrder(side, qty_);
    nextActTick_ = computeNextTick(tick);
}
