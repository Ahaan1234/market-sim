#include "OrderBook.h"
#include <algorithm>
#include <iostream>

uint64_t OrderBook::addLimitOrder(Side side, double price, double qty) {
    uint64_t id = nextId_++;
    if (side == Side::BUY)
        bids_[price] += qty;
    else
        asks_[price] += qty;
    return id;
}

std::vector<Trade> OrderBook::addMarketOrder(Side side, double qty) {
    std::vector<Trade> trades;
    uint64_t aggressorId = nextId_++;

    auto fill = [&](auto& book) {
        double remaining = qty;
        auto it = book.begin();
        while (it != book.end() && remaining > 0.0) {
            double fillQty = std::min(remaining, it->second);
            trades.push_back({aggressorId, 0, it->first, fillQty});
            it->second -= fillQty;
            remaining  -= fillQty;
            if (it->second == 0.0)
                it = book.erase(it);
            else
                break;
        }
    };

    if (side == Side::BUY)
        fill(asks_);
    else
        fill(bids_);

    return trades;
}

double OrderBook::bestBid()  const { return bids_.empty() ? 0.0 : bids_.begin()->first; }
double OrderBook::bestAsk()  const { return asks_.empty() ? 0.0 : asks_.begin()->first; }
double OrderBook::spread()   const { return bestAsk() - bestBid(); }
size_t OrderBook::bidDepth() const { return bids_.size(); }
size_t OrderBook::askDepth() const { return asks_.size(); }

void OrderBook::printBook() const {
    std::cout << "=== ORDER BOOK ===\n";
    for (auto it = asks_.rbegin(); it != asks_.rend(); ++it)
        std::cout << "  ASK  " << it->first << "  qty=" << it->second << "\n";
    std::cout << "  --- spread: " << spread() << " ---\n";
    for (auto& [price, qty] : bids_)
        std::cout << "  BID  " << price << "  qty=" << qty << "\n";
    std::cout << "==================\n";
}
