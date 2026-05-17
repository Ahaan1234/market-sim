#pragma once
#include <random>
#include <string>
#include "OrderBook.h"

class Trader {
public:
    Trader(std::string id, double capital, unsigned int seed);
    virtual ~Trader() = default;

    virtual void act(OrderBook& book, double midPrice, uint64_t tick) = 0;

    const std::string& id() const;
    double capital() const;

protected:
    std::string  id_;
    double       capital_;
    std::mt19937 rng_;
};
