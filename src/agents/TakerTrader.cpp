#include "TakerTrader.h"

TakerTrader::TakerTrader(std::string id, double capital, unsigned int seed,
                         double actProb, double qty)
    : Trader(std::move(id), capital, seed)
    , actProb_(actProb)
    , qty_(qty)
    , probDist_(0.0, 1.0)
    , sideDist_(0, 1) {}

void TakerTrader::act(OrderBook& book, double midPrice, uint64_t tick) {
    (void)midPrice;
    (void)tick;
    if (probDist_(rng_) > actProb_)
        return;

    Side side = (sideDist_(rng_) == 0) ? Side::BUY : Side::SELL;
    book.addMarketOrder(side, qty_);
}
