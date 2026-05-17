#include "Trader.h"

Trader::Trader(std::string id, double capital, unsigned int seed)
    : id_(std::move(id)), capital_(capital), rng_(seed) {}

const std::string& Trader::id()      const { return id_; }
double             Trader::capital() const { return capital_; }
