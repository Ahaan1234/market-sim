#pragma once
#include <cstdint>
#include <functional>
#include <map>
#include <vector>
#include "Order.h"

class OrderBook {
public:
    uint64_t           addLimitOrder(Side side, double price, double qty);
    std::vector<Trade> addMarketOrder(Side side, double qty);

    double bestBid()  const;
    double bestAsk()  const;
    double spread()   const;
    size_t bidDepth() const;
    size_t askDepth() const;

    void printBook()  const;

    std::vector<std::pair<double,double>> bidLevels(size_t maxLevels) const;
    std::vector<std::pair<double,double>> askLevels(size_t maxLevels) const;

private:
    std::map<double, double, std::greater<double>> bids_;
    std::map<double, double>                       asks_;
    uint64_t nextId_ = 1;
};
