#pragma once
#include <cstdint>
#include <memory>
#include <random>
#include <vector>
#include "OrderBook.h"
#include "OrderQueue.h"
#include "PositionTracker.h"
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

    Simulation(Config cfg, OrderQueue& queue);

    void runTick();
    void run(int ticks);

    double           currentPrice() const;
    const OrderBook& book()         const;
    uint64_t         tickNumber()   const;

private:
    OrderBook                            book_;
    PriceEngine                          priceEngine_;
    std::vector<std::unique_ptr<Trader>> traders_;
    std::mt19937                         rng_;
    uint64_t                             tick_ = 0;

    OrderQueue&     orderQueue_;
    PositionTracker posTracker_;

    void emitTick();
    void drainOrderQueue();
    void emitFill(const std::string& trader_id, const std::string& order_id,
                  Side side, double price, double qty);
    void emitReject(const std::string& trader_id, const std::string& order_id,
                    const std::string& reason);
};
