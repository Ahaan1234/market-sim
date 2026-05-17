#pragma once
#include <cstdint>
#include "../Trader.h"

class WhaleTrader : public Trader {
public:
    WhaleTrader(std::string id, double capital, unsigned int seed,
                int interval = 200, int jitter = 50, double baseQty = 1.0);

    void act(OrderBook& book, double midPrice, uint64_t tick) override;

private:
    int      interval_;
    double   qty_;
    uint64_t nextActTick_;

    uint64_t computeNextTick(uint64_t currentTick);
    std::uniform_int_distribution<int> jitterDist_;
    std::uniform_int_distribution<int> sideDist_;
};
