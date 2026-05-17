#pragma once
#include "../Trader.h"

class TakerTrader : public Trader {
public:
    TakerTrader(std::string id, double capital, unsigned int seed,
                double actProb = 0.30, double qty = 1.0);

    void act(OrderBook& book, double midPrice, uint64_t tick) override;

private:
    double actProb_;
    double qty_;
    std::uniform_real_distribution<double> probDist_;
    std::uniform_int_distribution<int>     sideDist_;
};
