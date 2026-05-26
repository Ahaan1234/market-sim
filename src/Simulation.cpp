#include "Simulation.h"
#include "agents/MakerTrader.h"
#include "agents/TakerTrader.h"
#include "agents/WhaleTrader.h"
#include <algorithm>
#include <chrono>
#include <iostream>
#include <numeric>
#include <sstream>

Simulation::Simulation(Config cfg, OrderQueue& queue)
    : priceEngine_(cfg.startPrice, cfg.seed)
    , rng_(cfg.seed ^ 0xDEADBEEFu)
    , orderQueue_(queue)
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

    drainOrderQueue();
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

void Simulation::drainOrderQueue() {
    while (auto item = orderQueue_.pop()) {
        IncomingOrder& o = *item;

        if (o.qty <= 0.0) {
            emitReject(o.trader_id, o.order_id, "invalid_qty");
            continue;
        }
        if (o.type == OrderType::LIMIT && o.price <= 0.0) {
            emitReject(o.trader_id, o.order_id, "invalid_price");
            continue;
        }

        std::vector<Trade> trades;

        if (o.type == OrderType::MARKET) {
            trades = book_.addMarketOrder(o.side, o.qty);
        } else {
            book_.addLimitOrder(o.side, o.price, o.qty);
            // Limit orders don't fill immediately in current model
        }

        for (const Trade& t : trades) {
            posTracker_.recordFill(o.trader_id, o.side, t.price, t.quantity);
            emitFill(o.trader_id, o.order_id, o.side, t.price, t.quantity);
        }

        if (trades.empty() && o.type == OrderType::MARKET) {
            emitReject(o.trader_id, o.order_id, "insufficient_liquidity");
        }
    }
}

static long long nowSeconds() {
    return std::chrono::duration_cast<std::chrono::seconds>(
               std::chrono::system_clock::now().time_since_epoch()).count();
}

void Simulation::emitFill(const std::string& trader_id, const std::string& order_id,
                           Side side, double price, double qty) {
    std::ostringstream oss;
    oss << "{\"type\":\"fill\""
        << ",\"trader_id\":\"" << trader_id << "\""
        << ",\"order_id\":\""  << order_id  << "\""
        << ",\"side\":\""      << (side == Side::BUY ? "BUY" : "SELL") << "\""
        << ",\"price\":"       << price
        << ",\"qty\":"         << qty
        << ",\"ts\":"          << nowSeconds()
        << "}\n";
    std::cout << oss.str();
    std::cout.flush();
}

void Simulation::emitReject(const std::string& trader_id, const std::string& order_id,
                             const std::string& reason) {
    std::ostringstream oss;
    oss << "{\"type\":\"reject\""
        << ",\"trader_id\":\"" << trader_id << "\""
        << ",\"order_id\":\""  << order_id  << "\""
        << ",\"reason\":\""    << reason    << "\""
        << ",\"ts\":"          << nowSeconds()
        << "}\n";
    std::cout << oss.str();
    std::cout.flush();
}

void Simulation::emitTick() {
    auto now = std::chrono::system_clock::now();
    auto ts  = std::chrono::duration_cast<std::chrono::seconds>(
                   now.time_since_epoch()).count();

    double bid         = book_.bestBid();
    double ask         = book_.bestAsk();
    double spread      = book_.spread();
    double curPrice    = priceEngine_.currentPrice();

    auto bids = book_.bidLevels(5);
    auto asks = book_.askLevels(5);

    std::ostringstream oss;
    oss << "{\"type\":\"tick\""
        << ",\"tick\":"   << tick_
        << ",\"ts\":"     << ts
        << ",\"price\":"  << curPrice
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

    oss << "]";

    if (tick_ % 10 == 0) {
        auto positions = posTracker_.snapshot();
        oss << ",\"positions\":[";
        for (size_t i = 0; i < positions.size(); ++i) {
            const Position& p = positions[i];
            if (i > 0) oss << ",";
            oss << "{\"trader_id\":\""    << p.trader_id     << "\""
                << ",\"net_qty\":"        << p.net_qty
                << ",\"unrealised_pnl\":" << p.unrealisedPnL(curPrice)
                << ",\"realised_pnl\":"   << p.realised_pnl
                << "}";
        }
        oss << "]";
    }

    oss << "}\n";

    std::cout << oss.str();
    std::cout.flush();
}
