#include "Simulation.h"
#include "agents/MakerTrader.h"
#include "agents/TakerTrader.h"
#include "agents/WhaleTrader.h"
#include <algorithm>
#include <chrono>
#include <iostream>
#include <numeric>
#include <sstream>

Simulation::Simulation(Config cfg)
    : priceEngine_(cfg.startPrice, cfg.seed)
    , rng_(cfg.seed ^ 0xDEADBEEFu)
{
    std::mt19937 seedGen(cfg.seed + 1u);
    std::uniform_int_distribution<unsigned int> seedDist;

    for (int i = 0; i < cfg.numMakers; ++i)
        traders_.push_back(std::make_unique<MakerTrader>(
            "maker_" + std::to_string(i), 10000.0, seedDist(seedGen)));

    for (int i = 0; i < cfg.numTakers; ++i)
        traders_.push_back(std::make_unique<TakerTrader>(
            "taker_" + std::to_string(i), 5000.0, seedDist(seedGen)));

    for (int i = 0; i < cfg.numWhales; ++i)
        traders_.push_back(std::make_unique<WhaleTrader>(
            "whale_" + std::to_string(i), 100000.0, seedDist(seedGen)));
}

void Simulation::runTick() {
    book_.clear();
    priceEngine_.nextPrice();
    double mid = priceEngine_.currentPrice();

    std::vector<size_t> order(traders_.size());
    std::iota(order.begin(), order.end(), 0);
    std::shuffle(order.begin(), order.end(), rng_);

    for (size_t idx : order)
        traders_[idx]->act(book_, mid, tick_);

    emitTick();
    ++tick_;
}

void Simulation::run(int ticks) {
    for (int i = 0; i < ticks; ++i)
        runTick();
}

double           Simulation::currentPrice() const { return priceEngine_.currentPrice(); }
const OrderBook& Simulation::book()         const { return book_; }
uint64_t         Simulation::tickNumber()   const { return tick_; }

void Simulation::emitTick() const {
    auto now = std::chrono::system_clock::now();
    auto ts  = std::chrono::duration_cast<std::chrono::seconds>(
                   now.time_since_epoch()).count();

    double bid    = book_.bestBid();
    double ask    = book_.bestAsk();
    double spread = book_.spread();

    auto bids = book_.bidLevels(5);
    auto asks = book_.askLevels(5);

    std::ostringstream oss;
    oss << "{\"type\":\"tick\""
        << ",\"tick\":"   << tick_
        << ",\"ts\":"     << ts
        << ",\"price\":"  << priceEngine_.currentPrice()
        << ",\"bid\":"    << bid
        << ",\"ask\":"    << ask
        << ",\"spread\":" << spread
        << ",\"bid_depth\":[";

    for (size_t i = 0; i < bids.size(); ++i) {
        if (i > 0) oss << ",";
        oss << "{\"price\":" << bids[i].first << ",\"qty\":" << bids[i].second << "}";
    }

    oss << "],\"ask_depth\":[";

    for (size_t i = 0; i < asks.size(); ++i) {
        if (i > 0) oss << ",";
        oss << "{\"price\":" << asks[i].first << ",\"qty\":" << asks[i].second << "}";
    }

    oss << "]}\n";

    std::cout << oss.str();
    std::cout.flush();
}
