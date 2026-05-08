cmake_minimum_required(VERSION 3.16)
project(market-sim)

set(CMAKE_CXX_STANDARD 17)
set(CMAKE_CXX_STANDARD_REQUIRED ON)

add_compile_options(-Wall -Wextra -Wpedantic)

add_executable(market-sim src/main.cpp src/PriceEngine.cpp src/OrderBook.cpp)
