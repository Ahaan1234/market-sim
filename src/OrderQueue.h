#pragma once
#include <mutex>
#include <optional>
#include <queue>
#include <string>
#include "Order.h"

enum class OrderType { LIMIT, MARKET };

struct IncomingOrder {
    std::string trader_id;
    std::string order_id;
    Side        side;
    OrderType   type;
    double      price;  // ignored if type == MARKET
    double      qty;
};

class OrderQueue {
public:
    void push(IncomingOrder order) {
        std::lock_guard<std::mutex> lock(mutex_);
        queue_.push(std::move(order));
    }

    std::optional<IncomingOrder> pop() {
        std::lock_guard<std::mutex> lock(mutex_);
        if (queue_.empty()) return std::nullopt;
        auto front = std::move(queue_.front());
        queue_.pop();
        return front;
    }

private:
    std::queue<IncomingOrder> queue_;
    std::mutex                mutex_;
};
