#include "PriceEngine.h"

PriceEngine::PriceEngine(double startPrice, unsigned int seed)
    : price_(startPrice), rng_(seed), dist_(-0.01, 0.01) {}

double PriceEngine::nextPrice() {
    double r = dist_(rng_);
    double candidate = price_ * (1.0 + r);
    price_ = (candidate > kFloor) ? candidate : kFloor;
    return price_;
}

double PriceEngine::currentPrice() const {
    return price_;
}
