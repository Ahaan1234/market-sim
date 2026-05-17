#pragma once
#include "../Trader.h"

class MakerTrader : public Trader {
public:
    MakerTrader(std::string id, double capital, unsigned int seed,
                double offset = 0.002, double qty = 1.0);

    void act(OrderBook& book, double midPrice, uint64_t tick) override;

private:
    double offset_;
    double qty_;
};
