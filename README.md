# Example Microservices for `go-observability`

This project contains a set of three simple Go microservices (`frontend`, `product`, and `user`) that are fully instrumented using the [`go-observability`](https://github.com/app-obs/go) library.

It serves as a real-world, runnable example of how to use the library to achieve automatic log correlation, distributed tracing, and standardized error handling. This project is designed to be run against the [`example-observability-server`](https://github.com/app-obs/example-observability-server).

## Project Structure

-   **/frontend**: A service that acts as the entry point. It receives requests from the user and calls the other two services.
-   **/product**: A service that provides product information.
-   **/user**: A service that provides user information.

## Prerequisites

-   **Docker Engine** and **Docker Compose V2**.
-   A running instance of the **[example-observability-server](https://github.com/app-obs/example-observability-server)**. Please follow the setup instructions in that repository first.

## How to Run

The configuration for the services is managed in the `.env` file. The default values are set up to connect to the `example-observability-server` running on the same host.

1.  Navigate to this directory.
2.  Build and start all three services in the background:
    ```sh
    docker compose up --build -d
    ```

## How to Test

Once the services are running, you can send a request to the `frontend` service. This will trigger a distributed trace that flows through all three services.

```sh
# Send a request for a valid product ID
curl http://localhost:8085/product-detail?id=123

# Send a request for a "missing" product to see an error trace
curl http://localhost:8085/product-detail?id=missing-456
```

## Viewing the Results

After sending a few test requests, you can see the complete, correlated observability data in your Grafana instance (`http://localhost:3000`).

-   **Traces**: Navigate to `Drilldown -> Traces` to see the distributed trace for your request. You will see the parent span from the `frontend` service and the child spans from the `product` and `user` services.
-   **Logs**: Navigate to `Drilldown -> Logs`. When you select a trace in the trace view, the logs panel will automatically be filtered to show only the logs that belong to that specific trace.

## How to Stop

To stop and remove all the service containers, run:

```sh
docker compose down
```
