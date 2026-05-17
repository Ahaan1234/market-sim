#pragma once
#include <cstdint>
#include <memory>
#include <random>
#include <vector>
#include "OrderBook.h"
#include "PriceEngine.h"
#include "Trader.h"

class Simulation {
public:
    struct Config {
        int          numMakers   = 10;
        int          numTakers   = 20;
        int          numWhales   = 2;
        double       startPrice  = 100.0;
        unsigned int seed        = 42;
    };

    explicit Simulation(Config cfg);

    void runTick();
    void run(int ticks);

    double          currentPrice() const;
    const OrderBook& book()        const;
    uint64_t         tickNumber()  const;

private:
    OrderBook                             book_;
    PriceEngine                           priceEngine_;
    std::vector<std::unique_ptr<Trader>>  traders_;
    std::mt19937                          rng_;
    uint64_t                              tick_ = 0;

    void emitTick() const;
};
